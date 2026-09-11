package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
	"github.com/lesomnus/cxz/internal/supervisor"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/workspace"
	"github.com/lesomnus/payday/config"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
	"github.com/lesomnus/payday/grpcx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	api.UnimplementedSessionsServer
	mu                     sync.Mutex
	projectionMu           sync.Mutex
	root, agent, configDir string
	db                     *sql.DB
	manager                *workspace.Manager
}

func Socket(root string) string { return filepath.Join(root, "run", "daemon.sock") }
func Dial(root string) (*grpc.ClientConn, error) {
	return transport.Dial(root)
}
func Run(ctx context.Context, root, agent, configDir string) error {
	if e := core.Prepare(root); e != nil {
		return e
	}
	lock, e := core.Lock(filepath.Join(root, "daemon.lock"))
	if e != nil {
		return e
	}
	defer lock.Close()
	db, _, e := (config.DbConfig{Driver: "sqlite3", Dsn: (&url.URL{Scheme: "file", Path: filepath.Join(root, "cxz.db")}).String() + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", MaxOpenConns: 1}).Open(ctx)
	if e != nil {
		return e
	}
	defer db.Close()
	if e = os.Chmod(filepath.Join(root, "cxz.db"), 0600); e != nil {
		return e
	}
	var version int
	if e = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); e != nil {
		return e
	}
	if version > 1 {
		return fmt.Errorf("unsupported database version %d", version)
	}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, q := range []string{`CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY, manifest BLOB NOT NULL, create_id TEXT UNIQUE NOT NULL)`, `CREATE TABLE IF NOT EXISTS events(session_id TEXT NOT NULL, seq INTEGER NOT NULL, data BLOB NOT NULL, PRIMARY KEY(session_id,seq))`, `PRAGMA user_version=1`} {
		if _, e = tx.ExecContext(ctx, q); e != nil {
			return e
		}
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	s := &Server{root: root, agent: agent, configDir: configDir, db: db}
	if os.Getenv("CXZ_OWNER") != "" {
		s.manager, e = workspace.New(db, root)
		if e != nil {
			return e
		}
	}
	// Manifests survive rebuilding the derived SQLite database.
	dirs, e := os.ReadDir(filepath.Join(root, "sessions"))
	if e != nil {
		return e
	}
	for _, d := range dirs {
		if !d.IsDir() || !validID.MatchString(d.Name()) {
			continue
		}
		b, e := os.ReadFile(filepath.Join(core.Dir(root, d.Name()), "session.json"))
		if e != nil {
			return e
		}
		var m core.Session
		if e = json.Unmarshal(b, &m); e != nil {
			return e
		}
		if m.ID != d.Name() {
			return errors.New("manifest id mismatch")
		}
		createID := m.CreateID
		if createID == "" {
			createID = "recovered:" + m.ID
		}
		if _, e = db.ExecContext(ctx, "INSERT OR IGNORE INTO sessions VALUES(?,?,?)", m.ID, b, createID); e != nil {
			return e
		}
	}
	sock := Socket(root)
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
	ctx, telemetry, e := (&config.OtelConfig{}).Build(ctx, config.Service{Name: "cxz", Scope: "github.com/lesomnus/cxz"})
	if e != nil {
		return e
	}
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = telemetry.Shutdown(shutdown)
	}()
	if e = telemetry.Start(ctx); e != nil {
		return e
	}
	opts := grpcx.ServerOptions(ctx)
	opts = append(opts, grpc.MaxRecvMsgSize(1024*1024), grpc.MaxSendMsgSize(20*1024*1024))
	g := grpc.NewServer(opts...)
	api.RegisterSessionsServer(g, s)
	if os.Getenv("CXZ_PROJECT_ID") != "" {
		runtime, e := workspace.LoadRuntime(root)
		if e != nil {
			return e
		}
		tcp, e := net.Listen("tcp", "0.0.0.0:7348")
		if e != nil {
			return e
		}
		defer tcp.Close()
		projectOptions := grpcx.ServerOptions(ctx)
		projectOptions = append(projectOptions, grpc.MaxRecvMsgSize(1024*1024), grpc.MaxSendMsgSize(24*1024*1024), grpc.ChainUnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			if e := transport.RequireToken(ctx, runtime.Token); e != nil {
				return nil, status.Error(codes.Unauthenticated, e.Error())
			}
			return handler(ctx, req)
		}), grpc.ChainStreamInterceptor(func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			if e := transport.RequireToken(stream.Context(), runtime.Token); e != nil {
				return status.Error(codes.Unauthenticated, e.Error())
			}
			return handler(srv, stream)
		}))
		projectServer := grpc.NewServer(projectOptions...)
		api.RegisterSessionsServer(projectServer, s)
		defer projectServer.Stop()
		go projectServer.Serve(tcp)
	}
	go func() { <-ctx.Done(); g.Stop() }()
	e = g.Serve(ln)
	if ctx.Err() != nil {
		return nil
	}
	return e
}

var validID = regexp.MustCompile(`^[a-f0-9]{24}$`)

func (s *Server) manifest(ctx context.Context, id string) (core.Session, error) {
	var m core.Session
	if !validID.MatchString(id) {
		return m, status.Error(codes.InvalidArgument, "invalid session id")
	}
	var b []byte
	if e := s.db.QueryRowContext(ctx, "SELECT manifest FROM sessions WHERE id=?", id).Scan(&b); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return m, status.Error(codes.NotFound, "session not found")
		}
		return m, e
	}
	return m, json.Unmarshal(b, &m)
}
func pbEvent(e core.Event) *api.Event {
	return &api.Event{SessionId: e.SessionID, RunId: e.RunID, Seq: e.Seq, TimeMs: e.TimeMS, Kind: e.Kind, Text: e.Text, RequestId: e.RequestID, Payload: e.Payload}
}
func (s *Server) snapshot(ctx context.Context, m core.Session) (*api.Session, error) {
	// Serialize read/advance so another reader cannot advance SQLite between this
	// reader's journal snapshot and its sequence consistency check.
	s.projectionMu.Lock()
	defer s.projectionMu.Unlock()
	events, e := journal.Read(filepath.Join(core.Dir(s.root, m.ID), "events.jsonl"))
	if e != nil {
		return nil, e
	}
	snap := supervisor.Replay(events)
	var last uint64
	if e = s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq),0) FROM events WHERE session_id=?", m.ID).Scan(&last); e != nil {
		return nil, e
	}
	if last > uint64(len(events)) {
		return nil, errors.New("journal is shorter than committed projection; refusing silent data loss")
	}
	if last < uint64(len(events)) {
		tx, e := s.db.BeginTx(ctx, nil)
		if e != nil {
			return nil, e
		}
		defer tx.Rollback()
		for _, v := range events[last:] {
			b, e := json.Marshal(v)
			if e != nil {
				return nil, e
			}
			if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO events VALUES(?,?,?)", m.ID, v.Seq, b); e != nil {
				return nil, e
			}
		}
		if e = tx.Commit(); e != nil {
			return nil, e
		}
	}
	var live core.Snapshot
	if supervisor.Call(ctx, s.root, m.ID, "status", nil, &live) == nil {
		snap = live
	} else {
		if snap.State != "stopped" && snap.State != "failed" {
			snap.State = "interrupted"
		}
		snap.Pending = nil
	}
	v := &api.Session{Id: m.ID, Workspace: m.Workspace, Title: m.Title, CreatedAt: m.CreatedAt, State: snap.State, RunId: snap.RunID, VendorId: snap.VendorID, LastSeq: snap.LastSeq, Agent: m.Kind, ProjectId: m.ProjectID}
	if v.Agent == "" {
		v.Agent = "claude"
	}
	for _, p := range snap.Pending {
		v.Pending = append(v.Pending, pbEvent(p))
	}
	return v, nil
}
func (s *Server) launch(ctx context.Context, m core.Session) (*api.Session, error) {
	exe, e := os.Executable()
	if e != nil {
		return nil, e
	}
	log, e := os.OpenFile(filepath.Join(core.Dir(s.root, m.ID), "supervisor.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	defer log.Close()
	cmd := exec.Command(exe, "--state", s.root, "_supervise", m.ID)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Start(); e != nil {
		return nil, e
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case e := <-done:
			return nil, fmt.Errorf("supervisor exited: %v; see private supervisor.log", e)
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, status.Error(codes.Unavailable, "supervisor starting; inspect session before retrying")
		case <-ticker.C:
			var v core.Snapshot
			if supervisor.Call(ctx, s.root, m.ID, "status", nil, &v) == nil && v.State != "starting" {
				return s.snapshot(ctx, m)
			}
		}
	}
}
func (s *Server) Create(ctx context.Context, r *api.CreateRequest) (*api.Session, error) {
	if s.manager != nil {
		return s.manager.Open(ctx, &api.ProjectRequest{Workspace: r.Workspace, Agent: r.Agent, NewSession: true, ClientId: r.ClientId})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.ClientId == "" || r.Workspace == "" {
		return nil, status.Error(codes.InvalidArgument, "workspace and client_id required")
	}
	if r.Agent == "" {
		r.Agent = "claude"
	}
	if r.Agent != "claude" && r.Agent != "codex" {
		return nil, status.Error(codes.InvalidArgument, "agent must be claude or codex")
	}
	path, e := filepath.Abs(r.Workspace)
	if e != nil {
		return nil, e
	}
	path, e = filepath.EvalSymlinks(path)
	if e != nil {
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	st, e := os.Stat(path)
	if e != nil || !st.IsDir() {
		return nil, status.Error(codes.InvalidArgument, "workspace must be a directory")
	}
	var b []byte
	e = s.db.QueryRowContext(ctx, "SELECT manifest FROM sessions WHERE create_id=?", r.ClientId).Scan(&b)
	if e == nil {
		var m core.Session
		if e = json.Unmarshal(b, &m); e != nil {
			return nil, e
		}
		if m.Workspace != path || m.Title != r.Title {
			return nil, status.Error(codes.AlreadyExists, "client_id reused")
		}
		return s.snapshot(ctx, m)
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	all, e := s.list(ctx)
	if e != nil {
		return nil, e
	}
	for _, m := range all {
		if m.Workspace == path {
			var live core.Snapshot
			if supervisor.Call(ctx, s.root, m.ID, "status", nil, &live) == nil {
				return nil, status.Error(codes.AlreadyExists, "workspace already has a live session: "+m.ID)
			}
		}
	}
	bin, cfg := s.agent, s.configDir
	if r.Agent == "codex" && os.Getenv("CXZ_PROJECT_ID") == "" {
		return nil, status.Error(codes.FailedPrecondition, "Codex requires an owned devcontainer: use cxz install and cxz up --agent codex")
	}
	if os.Getenv("CXZ_PROJECT_ID") != "" {
		runtime, e := workspace.LoadRuntime(s.root)
		if e != nil {
			return nil, e
		}
		if path != runtime.Workspace {
			return nil, status.Error(codes.PermissionDenied, "project runtime only serves its own workspace")
		}
		bin = runtime.Claude
		if r.Agent == "codex" {
			bin = runtime.Codex
		}
		if bin == "" {
			return nil, status.Error(codes.FailedPrecondition, "agent is not provisioned; run cxz up --agent "+r.Agent)
		}
		cfg = filepath.Join(s.root, "agents", r.Agent)
		if e = os.MkdirAll(cfg, 0700); e != nil {
			return nil, e
		}
	}
	m := core.Session{CreateID: r.ClientId, ID: core.ID(), Workspace: path, Title: r.Title, CreatedAt: time.Now().UnixMilli(), Agent: bin, Kind: r.Agent, ProjectID: os.Getenv("CXZ_PROJECT_ID"), ConfigDir: cfg}
	if e = os.Mkdir(core.Dir(s.root, m.ID), 0700); e != nil {
		return nil, e
	}
	if e = core.WriteJSON(filepath.Join(core.Dir(s.root, m.ID), "session.json"), m); e != nil {
		return nil, e
	}
	if e = core.SyncDir(filepath.Join(s.root, "sessions")); e != nil {
		return nil, e
	}
	b, _ = json.Marshal(m)
	if _, e = s.db.ExecContext(ctx, "INSERT INTO sessions VALUES(?,?,?)", m.ID, b, r.ClientId); e != nil {
		return nil, e
	}
	return s.launch(ctx, m)
}
func (s *Server) list(ctx context.Context) ([]core.Session, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT manifest FROM sessions ORDER BY rowid DESC")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var ms []core.Session
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		var m core.Session
		if e = json.Unmarshal(b, &m); e != nil {
			return nil, e
		}
		ms = append(ms, m)
	}
	return ms, rows.Err()
}
func (s *Server) List(ctx context.Context, _ *api.Empty) (*api.SessionList, error) {
	if s.manager != nil {
		return s.manager.Sessions(ctx)
	}
	ms, e := s.list(ctx)
	if e != nil {
		return nil, e
	}
	out := &api.SessionList{}
	for _, m := range ms {
		v, e := s.snapshot(ctx, m)
		if e != nil {
			return nil, e
		}
		out.Sessions = append(out.Sessions, v)
	}
	return out, nil
}
func (s *Server) Get(ctx context.Context, r *api.SessionRef) (*api.Session, error) {
	if s.manager != nil {
		return s.manager.Get(ctx, r.Id)
	}
	m, e := s.manifest(ctx, r.Id)
	if e != nil {
		return nil, e
	}
	return s.snapshot(ctx, m)
}
func (s *Server) command(ctx context.Context, id, op string, c core.Command) (*api.Receipt, error) {
	if _, e := s.manifest(ctx, id); e != nil {
		return nil, e
	}
	var v core.Receipt
	if e := supervisor.Call(ctx, s.root, id, op, c, &v); e != nil {
		return nil, status.Error(codes.FailedPrecondition, e.Error())
	}
	return &api.Receipt{ClientId: v.ClientID, Status: v.Status}, nil
}
func (s *Server) Send(ctx context.Context, r *api.Input) (*api.Receipt, error) {
	if s.manager != nil {
		c, client, e := s.manager.ClientFor(ctx, r.SessionId)
		if e != nil {
			return nil, e
		}
		defer c.Close()
		return client.Send(ctx, r)
	}
	return s.command(ctx, r.SessionId, "send", core.Command{RunID: r.RunId, ClientID: r.ClientId, Text: r.Text})
}
func (s *Server) Reply(ctx context.Context, r *api.Answer) (*api.Receipt, error) {
	if s.manager != nil {
		c, client, e := s.manager.ClientFor(ctx, r.SessionId)
		if e != nil {
			return nil, e
		}
		defer c.Close()
		return client.Reply(ctx, r)
	}
	var answers map[string]string
	if r.AnswersJson != "" {
		if e := json.Unmarshal([]byte(r.AnswersJson), &answers); e != nil {
			return nil, status.Error(codes.InvalidArgument, "answers_json must be a string map")
		}
	}
	return s.command(ctx, r.SessionId, "reply", core.Command{RunID: r.RunId, ClientID: r.ClientId, RequestID: r.RequestId, Allow: r.Allow, Answers: answers})
}
func (s *Server) Interrupt(ctx context.Context, r *api.Control) (*api.Receipt, error) {
	if s.manager != nil {
		c, client, e := s.manager.ClientFor(ctx, r.SessionId)
		if e != nil {
			return nil, e
		}
		defer c.Close()
		return client.Interrupt(ctx, r)
	}
	return s.command(ctx, r.SessionId, "interrupt", core.Command{RunID: r.RunId, ClientID: r.ClientId})
}
func (s *Server) Stop(ctx context.Context, r *api.Control) (*api.Receipt, error) {
	if s.manager != nil {
		c, client, e := s.manager.ClientFor(ctx, r.SessionId)
		if e != nil {
			return nil, e
		}
		defer c.Close()
		return client.Stop(ctx, r)
	}
	return s.command(ctx, r.SessionId, "stop", core.Command{RunID: r.RunId, ClientID: r.ClientId})
}
func (s *Server) Resume(ctx context.Context, r *api.Control) (*api.Session, error) {
	if s.manager != nil {
		c, client, e := s.manager.ClientFor(ctx, r.SessionId)
		if e != nil {
			return nil, e
		}
		defer c.Close()
		return client.Resume(ctx, r)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, e := s.manifest(ctx, r.SessionId)
	if e != nil {
		return nil, e
	}
	v, e := s.snapshot(ctx, m)
	if e != nil {
		return nil, e
	}
	if v.State == "starting" || v.State == "idle" || v.State == "working" || v.State == "waiting_input" {
		return v, nil
	}
	if r.RunId != v.RunId {
		return nil, status.Error(codes.FailedPrecondition, "stale run_id")
	}
	all, e := s.list(ctx)
	if e != nil {
		return nil, e
	}
	for _, other := range all {
		if other.ID != m.ID && other.Workspace == m.Workspace {
			var live core.Snapshot
			if supervisor.Call(ctx, s.root, other.ID, "status", nil, &live) == nil {
				return nil, status.Error(codes.AlreadyExists, "workspace has another live session")
			}
		}
	}
	return s.launch(ctx, m)
}
func (s *Server) Watch(r *api.WatchRequest, stream grpc.ServerStreamingServer[api.Event]) error {
	if s.manager != nil {
		return s.watchRemote(r, stream)
	}
	ctx := stream.Context()
	m, e := s.manifest(ctx, r.SessionId)
	if e != nil {
		return e
	}
	cursor := r.AfterSeq
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, e = s.snapshot(ctx, m); e != nil {
			return e
		}
		rows, e := s.db.QueryContext(ctx, "SELECT data FROM events WHERE session_id=? AND seq>? ORDER BY seq LIMIT 256", m.ID, cursor)
		if e != nil {
			return e
		}
		var batch []core.Event
		for rows.Next() {
			var b []byte
			if e = rows.Scan(&b); e != nil {
				rows.Close()
				return e
			}
			var v core.Event
			if e = json.Unmarshal(b, &v); e != nil {
				rows.Close()
				return e
			}
			batch = append(batch, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, v := range batch {
			if e = stream.Send(pbEvent(v)); e != nil {
				return e
			}
			cursor = v.Seq
		}
		if len(batch) == 256 {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
