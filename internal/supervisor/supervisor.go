// Package supervisor owns one Claude process independently of the daemon.
package supervisor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lesomnus/cxz/internal/accounts"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/filemap"
	"github.com/lesomnus/cxz/internal/journal"
)

type Supervisor struct {
	lastActivity     time.Time
	updateClients    map[string]updateClient
	updateTools      map[string]bool
	updateBackground agentview.BackgroundState
	updateUnknown    bool
	updateBlocked    bool
	mu               sync.Mutex
	session          core.Session
	log              *journal.Log
	snap             core.Snapshot
	pending          map[string]core.Event
	receipts         map[string]record
	cmd              *exec.Cmd
	stdin            io.WriteCloser
	done             chan struct{}
	stopping         bool
	interrupted      bool
	batch            []core.Event
	buffering        bool
	codex            *codexProtocol
	quotaSocket      string
	quotaChecking    bool
	quotaWorkers     sync.WaitGroup
	quotaFallbackAt  time.Time
	quotaRoot        string
	quotaToken       string
	quotaLease       string
	quotaObserved    int64
	quotaDisabled    bool
	quotaRequested   time.Time
	modelOptions     []agentview.ModelOption
	modelRequested   time.Time
	modelDisabled    bool
	modelSource      string
	modelPages       []agentview.ModelOption
	effort           string
	settingPending   string
}
type record struct {
	Op      string       `json:"op"`
	Command core.Command `json:"command"`
	Status  string       `json:"status"`
}

func Replay(events []core.Event) core.Snapshot {
	return ReplayFrom(core.Snapshot{State: "stopped", PermissionMode: "full"}, events)
}

// ReplayFrom advances a previously committed projection without revisiting its
// old events. Pending approvals are copied so callers retain an immutable snapshot.
func ReplayFrom(s core.Snapshot, events []core.Event) core.Snapshot {
	p := map[string]core.Event{}
	for _, e := range s.Pending {
		p[e.RequestID] = e
	}
	for _, e := range events {
		s.LastSeq = e.Seq
		switch e.Kind {
		case "permission":
			if e.Text == "ask" || e.Text == "full" {
				s.PermissionMode = e.Text
			}
		case "state":
			s.State = e.Text
			if s.RunID != e.RunID {
				p = map[string]core.Event{}
			}
			s.RunID = e.RunID
		case "vendor":
			s.VendorID = e.Text
		case "approval":
			p[e.RequestID] = e
		case "approval_resolved":
			delete(p, e.RequestID)
		}
	}
	s.Pending = nil
	for _, e := range p {
		s.Pending = append(s.Pending, e)
	}
	sort.Slice(s.Pending, func(i, j int) bool { return s.Pending[i].Seq < s.Pending[j].Seq })
	return s
}
func Run(ctx context.Context, root, id string) error {
	dir := core.Dir(root, id)
	lock, e := core.Lock(filepath.Join(dir, "supervisor.lock"))
	if e != nil {
		return e
	}
	defer lock.Close()
	var session core.Session
	b, e := os.ReadFile(filepath.Join(dir, "session.json"))
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &session); e != nil {
		return e
	}
	profile := accounts.SessionRoot(root, session.CreateID)
	if e = accounts.Prepare(profile, session.Account, session.Kind); e != nil {
		return e
	}
	profileLock, e := core.Lock(filepath.Join(accounts.Dir(profile, session.Account), "login.lock"))
	if e != nil {
		return e
	}
	defer profileLock.Close()
	l, e := journal.Open(filepath.Join(dir, "events.jsonl"))
	if e != nil {
		return e
	}
	defer l.Close()
	old := Replay(l.All())
	s := &Supervisor{session: session, log: l, snap: core.Snapshot{State: "starting", RunID: core.ID(), VendorID: old.VendorID, PermissionMode: old.PermissionMode}, pending: map[string]core.Event{}, receipts: map[string]record{}, done: make(chan struct{})}
	for _, v := range l.All() {
		if v.Kind == "setting" {
			s.restoreSetting(v.Text, v.Payload)
		}
		if v.Kind == "intent" || v.Kind == "receipt" || (v.Kind == "permission" && v.RequestID != "") {
			var r record
			if json.Unmarshal(v.Payload, &r) == nil {
				s.receipts[r.Command.ClientID] = r
			}
		}
	}
	defer s.quotaWorkers.Wait()
	s.quotaRoot = root
	var runtime struct{ Token string }
	if raw, err := os.ReadFile(filepath.Join(filepath.Dir(root), "runtime.json")); err == nil {
		_ = json.Unmarshal(raw, &runtime)
		s.quotaToken = runtime.Token
	}
	s.event("state", "starting", "", nil, nil)
	args := claudeArgs()
	if old.VendorID != "" {
		args = append(args, "--resume", old.VendorID)
	}
	if s.session.Model != "" {
		args = append(args, "--model", s.session.Model)
	}
	if s.effort != "" {
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "--settings" {
				var settings map[string]any
				_ = json.Unmarshal([]byte(args[i+1]), &settings)
				settings["effortLevel"] = s.effort
				b, _ := json.Marshal(settings)
				args[i+1] = string(b)
			}
		}
	}
	if session.Kind == "codex" {
		// Codex does not persist an empty thread until its first turn. Starting
		// a fresh empty thread is safe only when no send intent ever existed.
		hasInput := false
		for _, r := range s.receipts {
			if r.Op == "send" && !isSettingCommand(r.Command.Text) {
				hasInput = true
			}
		}
		if !hasInput {
			s.snap.VendorID = ""
		}
		s.codex = &codexProtocol{s: s}
		args = []string{"app-server", "--listen", "stdio://"}
	}
	auth, err := accounts.LaunchSession(root, session, os.Environ())
	if err != nil {
		return err
	}
	home := filepath.Join(accounts.Dir(accounts.SessionRoot(root, session.CreateID), session.Account), "home")
	if err := filemap.Apply(root, session.Kind, auth.ConfigDir, home, session.Workspace); err != nil {
		return err
	}
	args = append(args, auth.Args...)
	if session.AuthBackend == accounts.BrokeredAccessToken {
		s.codex.token = func(previous string, refresh bool) (accounts.Token, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			return accounts.FetchToken(ctx, root, session.Account, session.ProjectID, session.AuthBinding, previous, refresh)
		}
	}
	s.cmd = exec.Command(session.Agent, args...)
	s.cmd.Dir = session.Workspace
	s.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Pdeathsig: syscall.SIGKILL}
	s.cmd.Env = auth.Env
	s.stdin, e = s.cmd.StdinPipe()
	if e != nil {
		return e
	}
	out, e := s.cmd.StdoutPipe()
	if e != nil {
		return e
	}
	// Diagnostics stay local and private. They are never sent through the RPC logs.
	errlog, e := os.OpenFile(filepath.Join(dir, "agent.stderr.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer errlog.Close()
	s.cmd.Stderr = errlog
	if e = s.cmd.Start(); e != nil {
		s.event("state", "failed", "", nil, nil)
		return e
	}
	defer s.kill()
	// A separate pipe watcher survives supervisor SIGKILL and reaps the entire
	// agent process group, including active shell children (Pdeathsig alone cannot).
	disarm, e := guard(s.cmd.Process.Pid, errlog, profileLock)
	if e != nil {
		return e
	}
	defer disarm()
	sock := core.Socket(root, id)
	if e = os.Remove(sock); e != nil && !os.IsNotExist(e) {
		return e
	}
	ln, e := net.Listen("unix", sock)
	if e != nil {
		return e
	}
	defer ln.Close()
	defer os.Remove(sock)
	if e = os.Chmod(sock, 0600); e != nil {
		return e
	}
	if e = core.WriteJSON(filepath.Join(dir, "pid.json"), map[string]int{"supervisor": os.Getpid(), "agent": s.cmd.Process.Pid}); e != nil {
		return e
	}
	srv := &http.Server{Handler: http.HandlerFunc(s.serve), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	defer srv.Close()
	go srv.Serve(ln)
	go func() {
		defer close(s.done)
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for sc.Scan() {
			s.consume(append([]byte(nil), sc.Bytes()...))
		}
		if sc.Err() != nil {
			s.kill()
		}
		waitErr := s.cmd.Wait()
		s.mu.Lock()
		defer s.mu.Unlock()
		s.clearPending()
		state := "interrupted"
		if s.stopping {
			state = "stopped"
		}
		if waitErr != nil && !s.stopping {
			state = "failed"
		}
		s.event("state", state, "", nil, nil)
	}()
	s.mu.Lock()
	if s.codex != nil {
		e = s.write(rpc("cxz-initialize", "initialize", map[string]any{"clientInfo": map[string]any{"name": "cxz", "version": "0.1.0"}, "capabilities": map[string]any{"experimentalApi": true}}))
	} else {
		e = s.write(map[string]any{"type": "control_request", "request_id": "initialize", "request": map[string]any{"subtype": "initialize", "hooks": map[string]any{}, "sdkMcpServers": []any{}}})
	}
	s.mu.Unlock()
	if e != nil {
		return e
	}
	quotaTick := time.NewTicker(time.Minute)
	defer quotaTick.Stop()
	sharedQuotaTick := time.NewTicker(5 * time.Second)
	defer sharedQuotaTick.Stop()
	for {
		select {
		case <-s.done:
			return nil
		case <-sharedQuotaTick.C:
			s.mu.Lock()
			if s.codex == nil && (s.snap.State == "idle" || s.snap.State == "working" || s.snap.State == "waiting_input") {
				s.readClaudeQuota()
			}
			s.mu.Unlock()
		case <-quotaTick.C:
			s.mu.Lock()
			s.expireSetting()
			if !s.quotaDisabled && (s.snap.State == "idle" || s.snap.State == "working" || s.snap.State == "waiting_input") {
				if s.codex != nil {
					s.codex.readQuota()
				} else {
					s.readClaudeQuota()
				}
			}
			if s.snap.State == "idle" {
				s.readModels()
			}
			s.mu.Unlock()
		case <-ctx.Done():
			s.kill()
			<-s.done
			return ctx.Err()
		}
	}
}
func (s *Supervisor) kill() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
	}
}

// Every state transition is durable before it is externally observable.
func (s *Supervisor) event(kind, text, id string, payload any, raw []byte) core.Event {
	s.observeUpdateEvent(kind, text, id)
	var b []byte
	if payload != nil {
		b, _ = json.Marshal(payload)
	}
	e := core.Event{SessionID: s.session.ID, RunID: s.snap.RunID, Kind: kind, Text: text, RequestID: id, Payload: b, Raw: raw}
	var err error
	if s.buffering {
		e.Seq = s.snap.LastSeq + 1
		e.TimeMS = time.Now().UnixMilli()
		s.batch = append(s.batch, e)
	} else {
		e, err = s.log.Append(e)
	}
	if err != nil {
		s.kill()
		panic(fmt.Sprintf("journal durability failure: %v", err))
	}
	s.snap.LastSeq = e.Seq
	if kind == "permission" {
		s.snap.PermissionMode = text
	}
	if kind == "state" {
		s.snap.State = text
	}
	return e
}
func (s *Supervisor) write(v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = s.stdin.Write(append(b, '\n'))
	return e
}
func (s *Supervisor) clearPending() {
	for id := range s.pending {
		s.event("approval_resolved", "canceled", id, nil, nil)
		delete(s.pending, id)
	}
}
func (s *Supervisor) consume(raw []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observeUpdateRaw(raw)
	s.buffering = true
	s.batch = nil
	defer func() {
		_, err := s.log.AppendBatch(s.batch)
		s.buffering = false
		s.batch = nil
		if err != nil {
			s.kill()
			panic(fmt.Sprintf("journal durability failure: %v", err))
		}
		s.approvePending()
	}()
	s.event("raw", "", "", nil, raw)
	if s.codex != nil {
		s.codex.consume(raw)
		return
	}
	var v struct {
		Type      string         `json:"type"`
		Subtype   string         `json:"subtype"`
		SessionID string         `json:"session_id"`
		RequestID string         `json:"request_id"`
		Request   map[string]any `json:"request"`
		Response  struct {
			RequestID string          `json:"request_id"`
			Subtype   string          `json:"subtype"`
			Response  json.RawMessage `json:"response"`
			Error     string          `json:"error"`
		} `json:"response"`
		Message struct {
			Model   string          `json:"model"`
			Usage   json.RawMessage `json:"usage"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
		Result  string `json:"result"`
		IsError bool   `json:"is_error"`
	}
	if json.Unmarshal(raw, &v) != nil {
		s.event("diagnostic", "invalid agent JSON", "", nil, nil)
		return
	}
	if v.SessionID != "" && v.SessionID != s.snap.VendorID {
		s.snap.VendorID = v.SessionID
		s.event("vendor", v.SessionID, "", nil, nil)
	}
	switch v.Type {
	case "rate_limit_event":
		s.event("usage", "rate_limit_event", "", json.RawMessage(raw), nil)
	case "system":
		if v.Subtype == "compact_boundary" {
			s.event("compact", "completed", "", json.RawMessage(raw), nil)
		}
	case "control_response":
		if v.Response.RequestID == "cxz-models" {
			if v.Response.Subtype == "success" {
				s.modelOptions = agentview.Models("claude", v.Response.Response)
				s.modelSource = "list_models"
				s.publishModels()
			} else {
				text := strings.ToLower(v.Response.Error)
				s.modelDisabled = strings.Contains(text, "unsupported") || strings.Contains(text, "unknown")
				s.event("models_status", "unavailable", "", nil, nil)
			}
			return
		}
		if strings.HasPrefix(v.Response.RequestID, "cxz-setting-") {
			s.finishSetting(strings.TrimPrefix(v.Response.RequestID, "cxz-setting-"), v.Response.Subtype == "success")
			return
		}
		if v.Response.RequestID == "cxz-quota" {
			s.quotaRequested = time.Time{}
			if v.Response.Subtype == "success" {
				s.publishClaudeQuota(v.Response.Response)
			} else if text := strings.ToLower(v.Response.Error); strings.Contains(text, "unsupported") || strings.Contains(text, "unknown control") {
				s.quotaDisabled = true
				s.event("usage_status", "unsupported", "", nil, nil)
			} else {
				s.event("usage_status", "error", "", nil, nil)
			}
			return
		}
		if v.Response.RequestID == "initialize" {
			if v.Response.Subtype == "success" {
				s.modelOptions = agentview.Models("claude", v.Response.Response)
				s.publishModels()
				s.event("state", "idle", "", nil, nil)
				s.readClaudeQuota()
				s.readModels()
			} else {
				s.event("state", "failed", "", nil, nil)
				s.kill()
			}
		}
	case "control_request":
		if v.Request["subtype"] != "can_use_tool" {
			_ = s.write(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "error", "request_id": v.RequestID, "error": "unsupported control request"}})
			return
		}
		name, _ := v.Request["tool_name"].(string)
		p := s.event("approval", name, v.RequestID, v.Request, nil)
		s.pending[v.RequestID] = p
		s.event("state", "waiting_input", "", nil, nil)
	case "control_cancel_request":
		delete(s.pending, v.RequestID)
		s.event("approval_resolved", "canceled", v.RequestID, nil, nil)
		if len(s.pending) == 0 && s.snap.State == "waiting_input" {
			s.event("state", "working", "", nil, nil)
		}
	case "assistant", "user":
		if v.Type == "assistant" && len(v.Message.Usage) > 0 {
			s.event("usage", "context/message", "", map[string]any{"model": v.Message.Model, "usage": v.Message.Usage}, nil)
		}
		var blocks []struct {
			Type      string `json:"type"`
			Text      string `json:"text"`
			Name      string `json:"name"`
			ID        string `json:"id"`
			Content   any    `json:"content"`
			Input     any    `json:"input"`
			ToolUseID string `json:"tool_use_id"`
			IsError   bool   `json:"is_error"`
		}
		if json.Unmarshal(v.Message.Content, &blocks) != nil {
			return
		}
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if v.Type == "assistant" {
					s.event("assistant", b.Text, "", nil, nil)
				}
			case "tool_use":
				s.event("tool_call", b.Name, b.ID, b.Input, nil)
			case "tool_result":
				s.event("tool_result", "", b.ToolUseID, map[string]any{"content": b.Content, "is_error": b.IsError, "tool_use_id": b.ToolUseID}, nil)
			}
		}
	case "result":
		s.clearPending()
		result := "completed"
		if v.IsError {
			result = "failed"
		}
		if s.interrupted {
			result = "interrupted"
		}
		s.interrupted = false
		s.event("turn_end", result, "", json.RawMessage(raw), nil)
		s.event("state", "idle", "", nil, nil)
		s.readClaudeQuota()
	}
}
func (s *Supervisor) execute(op string, c core.Command) (core.Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.executeLocked(op, c)
}

func (s *Supervisor) executeLocked(op string, c core.Command) (core.Receipt, error) {
	receipt := core.Receipt{ClientID: c.ClientID}
	if c.ClientID == "" {
		return receipt, errors.New("client_id required")
	}
	if old, ok := s.receipts[c.ClientID]; ok {
		a, _ := json.Marshal(old.Command)
		b, _ := json.Marshal(c)
		if old.Op != op || !bytes.Equal(a, b) {
			return receipt, errors.New("client_id reused with different command")
		}
		receipt.Status = old.Status
		return receipt, nil
	}
	if c.RunID != s.snap.RunID {
		return receipt, errors.New("stale run_id; refresh session")
	}
	if op == "permission" {
		return s.setPermission(c)
	}
	if op == "update-stop" {
		if reason := s.updateReason(time.Now(), s.processesQuiet()); reason != "" {
			return receipt, errors.New(reason)
		}
		op = "stop"
		s.event("update", "applying agent update", "", nil, nil)
	}
	if op == "send" {
		if isSettingCommand(c.Text) {
			return s.configure(c)
		}
		if s.settingPending != "" {
			return receipt, errors.New("model/effort update pending; wait for confirmation")
		}
	}
	var wire any
	if s.codex != nil {
		var err error
		wire, err = s.codex.command(op, c)
		if err != nil {
			return receipt, err
		}
	} else {
		switch op {
		case "send":
			if s.snap.State != "idle" || c.Text == "" {
				return receipt, errors.New("session must be idle and text nonempty")
			}
			wire = map[string]any{"type": "user", "session_id": "", "parent_tool_use_id": nil, "message": map[string]any{"role": "user", "content": c.Text}}
		case "reply":
			p, ok := s.pending[c.RequestID]
			if !ok {
				return receipt, errors.New("approval is stale or already resolved")
			}
			var request struct {
				Tool  string         `json:"tool_name"`
				Input map[string]any `json:"input"`
			}
			if e := json.Unmarshal(p.Payload, &request); e != nil {
				return receipt, e
			}
			response := map[string]any{"behavior": "deny", "message": "User denied this request."}
			if c.Allow {
				if request.Tool == "AskUserQuestion" {
					answerValues := c.Answers
					if len(c.Selections) > 0 {
						questions, err := agentview.Questions("claude", request.Tool, p.Payload)
						if err != nil {
							return receipt, err
						}
						values, err := agentview.NormalizeAnswers(questions, c.Selections)
						if err != nil {
							return receipt, err
						}
						answers, annotations := agentview.ClaudeAnswerAnnotations(questions, values)
						answerValues = answers
						if len(annotations) > 0 {
							request.Input["annotations"] = annotations
						}
					}
					qs, _ := request.Input["questions"].([]any)
					if len(qs) == 0 {
						return receipt, errors.New("question payload missing")
					}
					for _, q := range qs {
						m, _ := q.(map[string]any)
						key, _ := m["question"].(string)
						if answerValues[key] == "" {
							return receipt, fmt.Errorf("answer required for %q", key)
						}
					}
					request.Input["answers"] = answerValues
				}
				response = map[string]any{"behavior": "allow", "updatedInput": request.Input}
			}
			wire = map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": c.RequestID, "response": response}}
		case "interrupt":
			if s.snap.State != "working" && s.snap.State != "waiting_input" {
				return receipt, errors.New("no active turn")
			}
			wire = map[string]any{"type": "control_request", "request_id": c.ClientID, "request": map[string]any{"subtype": "interrupt"}}
		case "stop":
		default:
			return receipt, errors.New("unknown command")
		}
	}
	r := record{Op: op, Command: c, Status: "delivery_unknown"}
	asyncReply := op == "reply" && s.pending[c.RequestID].Text == agentview.CodexAsyncQuestion
	s.event("intent", op, c.ClientID, r, nil)
	s.receipts[c.ClientID] = r
	switch op {
	case "send":
		s.event("input", c.Text, c.ClientID, nil, nil)
		s.event("state", "working", "", nil, nil)
	case "reply":
		delete(s.pending, c.RequestID)
		s.event("approval_resolved", map[bool]string{true: "allowed", false: "denied"}[c.Allow], c.RequestID, nil, nil)
		if asyncReply {
			if c.Allow {
				s.event("state", "working", "", nil, nil)
			}
		} else if !s.hasBlockingPending() {
			s.event("state", "working", "", nil, nil)
		}
	case "interrupt":
		s.interrupted = true
		s.clearPending()
		s.event("state", "working", "", nil, nil)
	case "stop":
		s.stopping = true
		s.event("state", "stopping", "", nil, nil)
	}
	if wire != nil {
		if e := s.write(wire); e != nil {
			s.kill()
			return core.Receipt{ClientID: c.ClientID, Status: "delivery_unknown"}, nil
		}
	}
	r.Status = "accepted"
	s.event("receipt", op, c.ClientID, r, nil)
	s.receipts[c.ClientID] = r
	receipt.Status = r.Status
	return receipt, nil
}
func (s *Supervisor) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.serveUpdate(w, r) {
		return
	}
	if r.Method == "GET" && r.URL.Path == "/status" {
		s.mu.Lock()
		v := s.snap
		v.Pending = nil
		for _, p := range s.pending {
			v.Pending = append(v.Pending, p)
		}
		s.mu.Unlock()
		json.NewEncoder(w).Encode(v)
		return
	}
	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	var c core.Command
	if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024)).Decode(&c); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	v, e := s.execute(r.URL.Path[1:], c)
	if e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	json.NewEncoder(w).Encode(v)
	if r.URL.Path == "/stop" || r.URL.Path == "/update-stop" {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		s.kill()
	}
}
func Call(ctx context.Context, root, id, op string, in, out any) error {
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", core.Socket(root, id))
	}}
	defer tr.CloseIdleConnections()
	method := "GET"
	var body io.Reader
	if in != nil {
		b, e := json.Marshal(in)
		if e != nil {
			return e
		}
		body = bytes.NewReader(b)
		method = "POST"
	}
	req, e := http.NewRequestWithContext(ctx, method, "http://unix/"+op, body)
	if e != nil {
		return e
	}
	res, e := (&http.Client{Transport: tr, Timeout: 5 * time.Second}).Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("supervisor %s: %s", strconv.Itoa(res.StatusCode), b)
	}
	return json.NewDecoder(res.Body).Decode(out)
}
