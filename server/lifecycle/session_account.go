package lifecycle

import (
	"bytes"
	"context"
	"strings"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SwitchAccount rebinds a stopped session to another account of its agent.
//
// The account and auth binding edges are immutable to every generic write; this
// is the one path that changes them, and it changes them together, after the
// runtime has moved the provider history. A session that has never run has no
// runtime manifest and only its resource edges change.
func (s SessionServer) SwitchAccount(ctx context.Context, r *resource.SessionSwitchAccountRequest) (*resource.Session, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	if !r.HasRef() || !r.HasAccount() {
		return nil, status.Error(codes.InvalidArgument, "session and account required")
	}
	if err := s.effect(); err != nil {
		return nil, err
	}
	if err := s.sync(ctx); err != nil {
		return nil, err
	}
	get := func(ref *resource.SessionRef) (*resource.Session, error) {
		return s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build(), Account: resource.AccountSelect_builder{All: ptr(true)}.Build(), AuthBinding: resource.AuthBindingSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
	}
	v, err := get(r.GetRef())
	if err != nil {
		return nil, err
	}
	// Start holds this guard for the whole launch; a switch must not interleave.
	guard, _ := s.shared.sessionStarts.LoadOrStore(string(v.GetId()), make(chan struct{}, 1))
	lock := guard.(chan struct{})
	select {
	case lock <- struct{}{}:
		defer func() { <-lock }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if v, err = get(resource.SessionRef_builder{Id: v.GetId()}.Build()); err != nil {
		return nil, err
	}
	a, err := s.Next().Account().Get(ctx, resource.AccountGetRequest_builder{Ref: r.GetAccount(), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	if bytes.Equal(a.GetId(), v.GetAccount().GetId()) {
		return v, nil
	}
	if a.GetAgent() != v.GetAgent() {
		return nil, status.Error(codes.InvalidArgument, "the account belongs to another agent")
	}
	if _, err = accounts.Resolve(a.GetAgent(), a.GetAuthBackend()); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	switch v.GetStatus().GetState() {
	case "created", "stopped", "interrupted", "failed":
	default:
		return nil, status.Error(codes.FailedPrecondition, "stop the session before switching its account")
	}
	key, err := s.accountProfile(ctx, a)
	if err != nil {
		return nil, err
	}
	project := projectid.String(v.GetProject().GetId())
	s.shared.mu.Lock()
	binding, err := s.ensureBinding(ctx, project, key, a.GetAgent(), a.GetAuthBackend())
	s.shared.mu.Unlock()
	if err != nil {
		return nil, err
	}
	// A first start that failed may still have saved its runtime manifest
	// (see Start), so only a session that never asked for one skips the runtime.
	if v.GetStatus().GetState() != "created" {
		_, err = s.shared.runtime.SwitchSessionAccount(ctx, &api.SessionAccountInput{SessionId: v.GetRuntimeId(), Account: key, AuthBackend: a.GetAuthBackend(), AuthBinding: binding.GetBindingId()})
		if err != nil && !(awaitingFirstRun(v) && runtimeSessionMissing(err)) {
			return nil, err
		}
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	// The generated store has no setter for an immutable edge. Write both
	// columns at once, then publish the change through the store so watchers,
	// the version column and the audit trail see an ordinary update.
	if _, err = s.shared.db.ExecContext(ctx, `UPDATE session SET account_id=?, auth_binding_id=? WHERE id=?`, projectid.String(a.GetId()), projectid.String(binding.GetId()), projectid.String(v.GetId())); err != nil {
		return nil, err
	}
	if _, err = s.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: resource.SessionRef_builder{Id: v.GetId()}.Build(), DateUpdatedForce: ptr(true)}.Build()); err != nil {
		return nil, err
	}
	return get(resource.SessionRef_builder{Id: v.GetId()}.Build())
}

// The project runtime answers NotFound; the host manager routing to it answers
// with an untyped error before it can ask.
func runtimeSessionMissing(err error) bool {
	return status.Code(err) == codes.NotFound || strings.Contains(err.Error(), "session not found")
}
