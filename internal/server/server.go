package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/distribution"
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
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/supervisor"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/workspace"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/cxz/server/lifecycle"
	"github.com/lesomnus/payday/config"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
	"github.com/lesomnus/payday/grpcx"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	updateQueued map[string]string
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
		broker, err := accounts.StartBroker(root, accounts.BrokerSocket, func(ctx context.Context, account string) error {
			bin, err := distribution.Ensure(ctx, "/cxz/tools", "codex", "", true)
			if err != nil {
				return err
			}
			return accounts.RefreshManaged(ctx, root, account, bin)
		})
		if err != nil {
			return err
		}
		defer broker.Close()
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
	resourceDB, _, e := (config.DbConfig{Driver: "sqlite3", Dsn: (&url.URL{Scheme: "file", Path: filepath.Join(root, "resources.db")}).String() + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", MaxOpenConns: 1}).Open(ctx)
	if e != nil {
		return e
	}
	defer resourceDB.Close()
	if e = os.Chmod(filepath.Join(root, "resources.db"), 0600); e != nil {
		return e
	}
	resources, e := lifecycle.Build(ctx, resourceDB, s)
	if e != nil {
		return fmt.Errorf("build payday resource server: %w", e)
	}
	if s.manager == nil {
		observeCtx, stopObserve := context.WithCancel(ctx)
		observeDone := make(chan struct{})
		go func() {
			defer close(observeDone)
			layer, _ := resource.Find[lifecycle.Layer](resources)
			if err := observeJournals(observeCtx, root, func(id string) {
				work, cancel := context.WithTimeout(observeCtx, 3*time.Second)
				defer cancel()
				if err := layer.RefreshSession(work, id); err != nil && observeCtx.Err() == nil {
					fmt.Fprintln(os.Stderr, "session projection:", err)
				}
			}); err != nil && observeCtx.Err() == nil {
				fmt.Fprintln(os.Stderr, "journal observer:", err)
			}
		}()
		defer func() { stopObserve(); <-observeDone }()
	}
	if s.manager == nil {
		s.recoverAgentUpdates(ctx)
	}
	if s.manager != nil {
		updatesCtx, cancelUpdates := context.WithCancel(ctx)
		updatesDone := make(chan struct{})
		go func() { defer close(updatesDone); s.manager.RunUpdates(updatesCtx) }()
		defer func() { cancelUpdates(); <-updatesDone }()
	} else {
		recoveryCtx, cancelRecovery := context.WithCancel(ctx)
		recoveryDone := make(chan struct{})
		go func() { defer close(recoveryDone); s.runUpdateRecovery(recoveryCtx) }()
		defer func() { cancelRecovery(); <-recoveryDone }()
	}
	watchCtx, cancelWatch := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		layer, _ := resource.Find[lifecycle.Layer](resources)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				if err := layer.Reconcile(watchCtx); err != nil && watchCtx.Err() == nil {
					fmt.Fprintln(os.Stderr, "resource reconciliation:", err)
				}
			}
		}
	}()
	defer func() { cancelWatch(); <-watchDone }()
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
	resource.RegisterServer(g, resources)
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
		resource.RegisterServer(projectServer, resources)
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
	if e.Kind == "raw" && agentview.IsBackgroundEvent(e.Raw) {
		e.Kind, e.Payload = "background", e.Raw
	}
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
	v := &api.Session{Id: m.ID, Workspace: m.Workspace, Title: m.Title, CreatedAt: m.CreatedAt, State: snap.State, RunId: snap.RunID, VendorId: snap.VendorID, LastSeq: snap.LastSeq, Agent: m.Kind, ProjectId: m.ProjectID, Model: m.Model, CreateId: m.CreateID, Account: m.Account, AuthBackend: m.AuthBackend, AuthBinding: m.AuthBinding}
	if v.Agent == "" {
		v.Agent = "claude"
	}
	for _, p := range snap.Pending {
		v.Pending = append(v.Pending, pbEvent(p))
	}
	return v, nil
}
func authCheckError(err error, backend string, creationKeys ...string) error {
	st := status.New(codes.FailedPrecondition, err.Error())
	var missing *accounts.LoginRequired
	if backend == accounts.ProjectLocalOAuth && errors.As(err, &missing) {
		metadata := map[string]string{"account": missing.Account}
		if len(creationKeys) > 0 {
			metadata["session_key"] = creationKeys[0]
		}
		withDetails, e := st.WithDetails(&errdetails.ErrorInfo{Domain: "cxz.auth", Reason: "PROJECT_LOGIN_REQUIRED", Metadata: metadata})
		if e == nil {
			st = withDetails
		}
	}
	return st.Err()
}

func (s *Server) launch(ctx context.Context, m core.Session) (*api.Session, error) {
	if m.Account != "" {
		backend, err := accounts.ResolveBinding(m.Kind, m.AuthBackend, m.ProjectID, m.Account, m.AuthBinding)
		if err != nil {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		if err := backend.Check(accounts.AuthRoot(s.root, m), m.Account); err != nil {
			return nil, authCheckError(err, m.AuthBackend, m.CreateID)
		}
	}
	if err := s.awaitSessionRelease(ctx, m); err != nil {
		return nil, err
	}
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

// Stop can acknowledge before supervisor defers and its process-group guard
// release their leases. Wait only for this session, never the whole workspace.
// Run still acquires both leases itself; this is not a replacement for locking.
func (s *Server) awaitSessionRelease(ctx context.Context, m core.Session) error {
	profile := accounts.SessionRoot(s.root, m.CreateID)
	if err := accounts.Prepare(profile, m.Account, m.Kind); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		ready := true
		var held []*os.File
		for _, path := range []string{filepath.Join(core.Dir(s.root, m.ID), "supervisor.lock"), filepath.Join(accounts.Dir(profile, m.Account), "login.lock")} {
			f, err := core.Lock(path)
			if err != nil {
				ready = false
				break
			}
			held = append(held, f)
		}
		for _, f := range held {
			f.Close()
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return status.Error(codes.Unavailable, "session process/profile is still busy; retry after it finishes stopping or logging in")
		case <-time.After(25 * time.Millisecond):
		}
	}
}
func (s *Server) Create(ctx context.Context, r *api.CreateRequest) (*api.Session, error) {
	if s.manager != nil {
		return s.manager.CreateSession(ctx, r)
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
	if err := settings.ValidateModel(r.Model); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
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
		if m.Workspace != path || m.Title != r.Title || m.Kind != r.Agent || m.Model != r.Model || m.Account != r.Account || m.AuthBackend != r.AuthBackend || m.AuthBinding != r.AuthBinding {
			return nil, status.Error(codes.AlreadyExists, "client_id reused")
		}
		return s.snapshot(ctx, m)
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	bin := s.agent
	if r.Agent == "codex" && os.Getenv("CXZ_PROJECT_ID") == "" {
		return nil, status.Error(codes.FailedPrecondition, "Codex requires an owned devcontainer: use cxz install and cxz project up --agent codex")
	}
	if os.Getenv("CXZ_PROJECT_ID") != "" {
		if r.Account == "" {
			return nil, status.Error(codes.InvalidArgument, "account required; register and log in with cxz account")
		}
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
			return nil, status.Error(codes.FailedPrecondition, "agent is not provisioned; run cxz project up --agent "+r.Agent)
		}
	}
	if r.Account != "" {
		backend, err := accounts.Resolve(r.Agent, r.AuthBackend)
		if err != nil {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		if err := backend.Check(accounts.AuthRoot(s.root, core.Session{CreateID: r.ClientId, AuthBackend: r.AuthBackend}), r.Account); err != nil {
			return nil, authCheckError(err, r.AuthBackend, r.ClientId)
		}
	}
	project, err := s.RegisterProject(ctx, path, "")
	if err != nil {
		return nil, err
	}
	if _, err := accounts.ResolveBinding(r.Agent, r.AuthBackend, project.Id, r.Account, r.AuthBinding); err != nil {
		return nil, status.Error(codes.InvalidArgument, "auth binding does not belong to this project")
	}
	m := core.Session{CreateID: r.ClientId, ID: core.ID(), Workspace: path, Title: r.Title, CreatedAt: time.Now().UnixMilli(), Agent: bin, Kind: r.Agent, ProjectID: project.Id, Model: r.Model, Account: r.Account, AuthBackend: r.AuthBackend, AuthBinding: r.AuthBinding}
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
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commandUnlocked(ctx, id, op, c)
}
func (s *Server) commandUnlocked(ctx context.Context, id, op string, c core.Command) (*api.Receipt, error) {
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
	answers, selections, err := core.DecodeAnswers(r.AnswersJson)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return s.command(ctx, r.SessionId, "reply", core.Command{RunID: r.RunId, ClientID: r.ClientId, RequestID: r.RequestId, Allow: r.Allow, Answers: answers, Selections: selections})
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
		return s.manager.ResumeSession(ctx, r)
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
	var lastWatchActivity time.Time
	var lastJournalInfo os.FileInfo
	var lastJournalRefresh time.Time
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		// Legacy watchers cannot report input activity. Treat them as busy
		// until disconnected rather than restarting underneath an old TUI.
		if lastWatchActivity.IsZero() || (r.ClientId == "" && time.Since(lastWatchActivity) > 10*time.Second) {
			var snap core.Snapshot
			if supervisor.Call(ctx, s.root, m.ID, "status", nil, &snap) == nil {
				clientID := r.ClientId
				if clientID == "" {
					clientID = "legacy-watch"
				}
				_, _ = s.Activity(ctx, &api.ActivityInput{SessionId: m.ID, RunId: snap.RunID, ClientId: clientID, Busy: true})
			}
			lastWatchActivity = time.Now()
		}
		info, statErr := os.Stat(filepath.Join(core.Dir(s.root, m.ID), "events.jsonl"))
		if statErr == nil && lastJournalInfo != nil && os.SameFile(info, lastJournalInfo) && info.Size() == lastJournalInfo.Size() && info.ModTime() == lastJournalInfo.ModTime() && time.Since(lastJournalRefresh) < 30*time.Second {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				continue
			}
		}
		if _, e = s.snapshot(ctx, m); e != nil {
			return e
		}
		lastJournalInfo, lastJournalRefresh = info, time.Now()
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
			lastJournalInfo = nil // Drain a backlog without waiting for another write.
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
