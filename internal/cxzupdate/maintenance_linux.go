package cxzupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/observe"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Health struct {
	Build      Build               `json:"build"`
	Binary     string              `json:"binary"`
	Start      string              `json:"start"`
	Ready      bool                `json:"ready"`
	Reason     string              `json:"reason,omitempty"`
	Sessions   map[string]string   `json:"sessions,omitempty"`
	Runtime    *RuntimeTransaction `json:"runtime_update,omitempty"`
	Lease      string              `json:"lease,omitempty"`
	LeaseUntil time.Time           `json:"lease_until,omitempty"`
	Failure    *RecoveryFailure    `json:"failure,omitempty"`
}
type lease struct {
	ID    string
	Until time.Time
}
type Gate struct {
	mu   sync.RWMutex
	root string
	held lease
	err  error
}

func NewGate(root string) *Gate {
	g := &Gate{root: root}
	b, e := os.ReadFile(filepath.Join(root, "run", "update-lease.json"))
	if e == nil {
		g.err = json.Unmarshal(b, &g.held)
	}
	if e != nil && !os.IsNotExist(e) {
		g.err = e
	}
	return g
}
func (g *Gate) blocked() bool {
	return g.err != nil || g.held.ID != "" && time.Now().Before(g.held.Until)
}
func (g *Gate) Busy() bool { g.mu.RLock(); defer g.mu.RUnlock(); return g.blocked() }

// mutating decides what an update may refuse. Reading is always allowed: a
// conversation stays legible while its runtime is replaced, and the reader is
// not the thing an update has to serialize with.
//
// Fail closed, so a method added later is refused until it is classified. The
// cost of that mistake is an input rejected during an update; the cost of the
// other mistake is a write racing the replacement.
//
// Before cxz#204 a refusal was a wait rather than an answer -- the readiness
// sweep held the gate's lock while it ran -- so a read misfiled here only
// looked slow. Now it would fail, which is why the reads are named.
func mutating(method string) bool {
	name := method[strings.LastIndex(method, "/")+1:]
	switch name {
	case // The catalog and what a session is doing.
		"Get", "List", "Watch", "Events", "History", "Projects",
		// Reading a conversation, and the records behind a row.
		"Transcript", "Rows", "EventDetails", "Search", "Background",
		"GetHistoryFloor", "GetHistoryPolicy", "Models",
		// Reading what is configured or installed.
		"Paths", "Memory", "Logs", "McpLogs", "McpSessions", "GetMcpServers",
		"GetSkills", "FileMappings", "Devcontainer", "RenderDevcontainer",
		"EngineStatus", "GetEngineInfo", "GetInstallationVersion",
		// Reading what the manager has been doing: asking why an update is
		// stuck must not be refused by the update that is stuck.
		"Diagnostics",
		// Reading auxiliary state, which is a model task's own status.
		"AuxStatus", "AuxEvents", "AuxConfig", "AuxModels", "AuxLoginInfo":
		return false
	}
	return true
}
func (g *Gate) Unary(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	if !mutating(info.FullMethod) {
		return next(ctx, req)
	}
	// Named, because a call that is slow and a call that is waiting for an
	// update look the same from the outside. See cxz#211.
	observe.Wait(ctx, "update gate", g.mu.RLock)
	defer g.mu.RUnlock()
	if g.blocked() {
		return nil, status.Error(codes.Unavailable, "cxz update in progress; input was not accepted")
	}
	return next(ctx, req)
}
func (g *Gate) Stream(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
	if !mutating(info.FullMethod) {
		return next(srv, stream)
	}
	observe.Wait(stream.Context(), "update gate", g.mu.RLock)
	defer g.mu.RUnlock()
	if g.blocked() {
		return status.Error(codes.Unavailable, "cxz update in progress")
	}
	return next(srv, stream)
}

// hold claims the server for an update, then checks that the server is quiet
// enough to take it. That order matters: the check asks every project, and
// every session within it, whether it is mid-turn, which takes as long as the
// slowest of them.
//
// Holding the gate's own lock across that check made every mutating RPC wait
// for it -- not refused, waited -- so a sweep that took ten seconds stopped the
// server for ten seconds, every time the rollout ticked. The lease refuses
// input by itself, and refusing is an answer a client can act on. See cxz#204.
func (g *Gate) hold(id string, check func() error) error {
	if len(id) != 24 || strings.ContainsAny(id, "/\\ \n") {
		return RecoveryBlocked("invalid_transaction", "invalid update transaction")
	}
	g.mu.Lock()
	if g.err != nil {
		g.mu.Unlock()
		return RecoveryBlocked("invalid_lease", fmt.Sprintf("invalid maintenance lease: %v", g.err))
	}
	if g.blocked() && g.held.ID != id {
		g.mu.Unlock()
		return RecoveryBlocked("foreign_lease", "another update owns this server")
	}
	l := lease{ID: id, Until: time.Now().Add(10 * time.Minute)}
	if e := core.WriteJSON(filepath.Join(g.root, "run", "update-lease.json"), l); e != nil {
		g.mu.Unlock()
		return e
	}
	g.held = l
	g.mu.Unlock()
	// From here input is refused, so nothing is waiting on this check.
	if e := check(); e != nil {
		_ = RecordAttempt(g.root, Attempt{At: time.Now(), Lease: id, Outcome: "refused", Reason: e.Error()})
		if x := g.release(id); x != nil {
			return fmt.Errorf("%w (and the lease could not be released: %v)", e, x)
		}
		return e
	}
	_ = RecordAttempt(g.root, Attempt{At: time.Now(), Lease: id, Outcome: "granted"})
	return nil
}
func (g *Gate) release(id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.held.ID != "" && g.held.ID != id {
		return fmt.Errorf("update lease owner mismatch")
	}
	if e := os.Remove(filepath.Join(g.root, "run", "update-lease.json")); e != nil && !os.IsNotExist(e) {
		return e
	}
	g.held = lease{}
	return nil
}
func ProcessStart(pid int) (string, error) {
	b, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if e != nil {
		return "", e
	}
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 {
		return "", fmt.Errorf("invalid process stat")
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 20 {
		return "", fmt.Errorf("invalid process stat")
	}
	if f[0] == "Z" {
		return "", os.ErrNotExist
	}
	return f[19], nil
}
func Maintenance(ctx context.Context, root string, g *Gate, check func(context.Context) (map[string]string, error)) (io.Closer, error) {
	path := filepath.Join(root, "run", "update.sock")
	_ = os.Remove(path)
	ln, e := net.Listen("unix", path)
	if e != nil {
		return nil, e
	}
	if e = os.Chmod(path, 0600); e != nil {
		ln.Close()
		return nil, e
	}
	binary, e := PinSelf(root)
	if e != nil {
		ln.Close()
		return nil, e
	}
	start, e := ProcessStart(os.Getpid())
	if e != nil {
		ln.Close()
		return nil, e
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var q struct{ Action, Transaction string }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&q) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		h := Health{Build: Current(), Binary: binary, Start: start}
		if tx, err := readRuntimeTransaction(root); err == nil {
			h.Runtime = &tx
		}
		var e error
		switch q.Action {
		case "health":
		case "status", "prepare", "prepare-force":
			inspect := func() error {
				var e error
				h.Sessions, e = check(r.Context())
				h.Ready = e == nil
				if e != nil {
					h.Reason = e.Error()
				}
				return e
			}
			switch q.Action {
			case "prepare":
				e = g.hold(q.Transaction, inspect)
			case "prepare-force":
				// Admission still closes -- a replacement must not run beside
				// ordinary work -- but the readiness sweep stops being a veto.
				// Somebody is waiting on this rollout, which outranks a session
				// that says it is busy. See cxz#253.
				e = g.hold(q.Transaction, func() error {
					h.Sessions, _ = check(r.Context())
					h.Ready = true
					h.Reason = "forced"
					return nil
				})
			default:
				e = inspect()
			}
		case "release":
			e = g.release(q.Transaction)
		default:
			http.Error(w, "invalid action", 400)
			return
		}
		// Read lease separately; checks may take the server's command lock.
		g.mu.RLock()
		if g.blocked() && g.err == nil {
			h.Lease = g.held.ID
			h.LeaseUntil = g.held.Until
		}
		g.mu.RUnlock()
		if e != nil {
			h.Reason = e.Error()
			h.Failure = RecoveryProblem(e)
			if q.Action != "status" {
				w.WriteHeader(409)
			}
		}
		_ = json.NewEncoder(w).Encode(h)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(ln)
	go func() { <-ctx.Done(); server.Close() }()
	return server, nil
}
func Call(ctx context.Context, root, action, transaction string) (Health, error) {
	var h Health
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(root, "run", "update.sock"))
	}}
	defer tr.CloseIdleConnections()
	b, _ := json.Marshal(map[string]string{"Action": action, "Transaction": transaction})
	req, e := http.NewRequestWithContext(ctx, "POST", "http://local/", bytes.NewReader(b))
	if e != nil {
		return h, e
	}
	res, e := (&http.Client{Transport: tr, Timeout: 30 * time.Second}).Do(req)
	if e != nil {
		return h, e
	}
	defer res.Body.Close()
	if e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&h); e != nil {
		return h, e
	}
	if res.StatusCode != 200 {
		if h.Failure != nil {
			return h, h.Failure
		}
		return h, fmt.Errorf("maintenance: %s", h.Reason)
	}
	return h, nil
}

// Run serializes internal mutations with maintenance admission, just like RPCs.
func (g *Gate) Run(fn func()) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.blocked() {
		return false
	}
	fn()
	return true
}

func leaseOwned(root, id string) bool {
	g := NewGate(root)
	return g.err == nil && g.blocked() && g.held.ID == id
}
