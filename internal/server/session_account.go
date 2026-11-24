package server

import (
	"context"
	"path/filepath"

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
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.manifest(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	var live core.Snapshot
	if supervisor.Call(ctx, s.root, m.ID, "status", nil, &live) == nil {
		return nil, status.Error(codes.FailedPrecondition, "stop the session before switching its account")
	}
	lock, err := core.Lock(filepath.Join(core.Dir(s.root, m.ID), "supervisor.lock"))
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, "session is starting or stopping; retry when settled")
	}
	defer lock.Close()
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
