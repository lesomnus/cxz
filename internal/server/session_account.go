package server

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionaccount"
	"github.com/lesomnus/cxz/internal/sessionstore"
	"github.com/lesomnus/cxz/internal/supervisor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SwitchSessionAccount rebinds a stopped session. A running supervisor has the
// old account's configuration directory as its agent's home, so the session
// must be stopped first rather than having its history moved out from under it.
func (s *Server) SwitchSessionAccount(ctx context.Context, r *api.SessionAccountInput) (*api.Session, error) {
	if s.manager != nil {
		return s.manager.SwitchSessionAccount(ctx, r)
	}
	if r.Account == "" || r.AuthBinding == "" {
		return nil, status.Error(codes.InvalidArgument, "account and auth_binding required")
	}
	m, err := s.manifest(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	lock, err := s.settledLease(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	// The lease keeps a supervisor from starting; s.mu orders this against the
	// other manifest writers. It is taken after the lease, which may be waited
	// for, so a stopping session never holds up the rest of the runtime.
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, err = s.manifest(ctx, r.SessionId); err != nil {
		return nil, err
	}
	// The supervisor is not running, so nothing appends to the store while it
	// is read; the lease keeps one from starting until the manifest is saved.
	store, err := sessionstore.Open(ctx, sessionstore.Path(core.Dir(s.root, m.ID)), m.ID, false)
	if err != nil {
		return nil, err
	}
	events, err := store.Events(ctx)
	store.Close()
	if err != nil {
		return nil, err
	}
	snapshot := supervisor.Replay(events)
	switch snapshot.State {
	case "", "idle", "stopped", "interrupted", "failed":
	default:
		return nil, status.Error(codes.FailedPrecondition, "switching accounts requires a settled conversation")
	}
	if len(snapshot.Pending) > 0 || snapshot.Queued != "" {
		return nil, status.Error(codes.FailedPrecondition, "resolve pending questions and queued input before switching accounts")
	}
	next, err := sessionaccount.Switch(s.root, m, sessionaccount.Target{Account: r.Account, AuthBackend: r.AuthBackend, AuthBinding: r.AuthBinding})
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	if next != m {
		if err = s.saveAgentManifest(ctx, next); err != nil {
			return nil, err
		}
	}
	return s.snapshot(ctx, next)
}

// settledLease takes the supervisor lock of a session that is not running. A
// Stop answers once the agent is told to exit, a moment before its supervisor
// releases the lock, so a switch made right after a Stop -- which is what the
// web UI does -- waits that moment out instead of failing.
func (s *Server) settledLease(ctx context.Context, id string) (*os.File, error) {
	path := filepath.Join(core.Dir(s.root, id), "supervisor.lock")
	deadline := time.Now().Add(settleWait)
	for {
		var live core.Snapshot
		if supervisor.Call(ctx, s.root, id, "status", nil, &live) == nil {
			return nil, status.Error(codes.FailedPrecondition, "stop the session before switching its account")
		}
		lock, err := core.Lock(path)
		if err == nil {
			return lock, nil
		}
		if time.Now().After(deadline) {
			return nil, status.Error(codes.FailedPrecondition, "session is starting or stopping; retry when settled")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// How long a switch waits for a stopping supervisor to let go of its lock.
const settleWait = 10 * time.Second
