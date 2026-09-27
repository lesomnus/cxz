package cxzupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
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
	Build    Build               `json:"build"`
	Binary   string              `json:"binary"`
	Start    string              `json:"start"`
	Ready    bool                `json:"ready"`
	Reason   string              `json:"reason,omitempty"`
	Sessions map[string]string   `json:"sessions,omitempty"`
	Runtime  *RuntimeTransaction `json:"runtime_update,omitempty"`
	Lease    string              `json:"lease,omitempty"`
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
func mutating(method string) bool {
	name := method[strings.LastIndex(method, "/")+1:]
	switch name {
	case "Get", "List", "Watch", "Events", "History", "Background", "Paths", "Memory", "Logs":
		return false
	}
	return true
}
func (g *Gate) Unary(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	if !mutating(info.FullMethod) {
		return next(ctx, req)
	}
	g.mu.RLock()
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
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.blocked() {
		return status.Error(codes.Unavailable, "cxz update in progress")
	}
	return next(srv, stream)
}
func (g *Gate) hold(id string, check func() error) error {
	if len(id) != 24 || strings.ContainsAny(id, "/\\ \n") {
		return fmt.Errorf("invalid update transaction")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.err != nil {
		return fmt.Errorf("invalid maintenance lease: %w", g.err)
	}
	if g.blocked() && g.held.ID != id {
		return fmt.Errorf("another update owns this server")
	}
	if e := check(); e != nil {
		return e
	}
	l := lease{ID: id, Until: time.Now().Add(10 * time.Minute)}
	if e := core.WriteJSON(filepath.Join(g.root, "run", "update-lease.json"), l); e != nil {
		return e
	}
	g.held = l
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
		case "status", "prepare":
			inspect := func() error {
				var e error
				h.Sessions, e = check(r.Context())
				h.Ready = e == nil
				if e != nil {
					h.Reason = e.Error()
				}
				return e
			}
			if q.Action == "prepare" {
				e = g.hold(q.Transaction, inspect)
			} else {
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
		}
		g.mu.RUnlock()
		if e != nil {
			h.Reason = e.Error()
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
