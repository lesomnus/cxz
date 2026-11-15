package lifecycle

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func awaitingFirstRun(v *resource.Session) bool {
	state := v.GetStatus()
	return state.GetRunId() == "" && (state.GetState() == "created" || state.GetState() == "starting" || state.GetState() == "failed")
}

// A stored session has no journal until Start. Keep its readers connected,
// without asking a project runtime to read a manifest that does not exist yet.
func (s SessionServer) awaitFirstRun(ctx context.Context, v *resource.Session, heartbeat func() error) (*resource.Session, error) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	pulse := time.NewTicker(10 * time.Second)
	defer pulse.Stop()
	for awaitingFirstRun(v) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-pulse.C:
			if err := heartbeat(); err != nil {
				return nil, err
			}
		case <-tick.C:
			var err error
			v, err = s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{Ref: resource.SessionRef_builder{Id: v.GetId()}.Build(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
			if err != nil {
				return nil, err
			}
		}
	}
	return v, nil
}

func (s SessionServer) Start(ctx context.Context, r *resource.SessionControl) (_ *resource.Session, result error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	if err := s.effect(); err != nil {
		return nil, err
	}
	get := func(ref *resource.SessionRef) (*resource.Session, error) {
		return s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build(), Account: resource.AccountSelect_builder{All: ptr(true)}.Build(), AuthBinding: resource.AuthBindingSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
	}
	v, err := get(r.GetRef())
	if status.Code(err) == codes.NotFound {
		if err = s.ensureSnapshot(ctx); err == nil {
			v, err = get(r.GetRef())
		}
	}
	if err != nil {
		return nil, err
	}
	guard, _ := s.shared.sessionStarts.LoadOrStore(string(v.GetId()), make(chan struct{}, 1))
	lock := guard.(chan struct{})
	select {
	case lock <- struct{}{}:
		defer func() { <-lock }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	v, err = get(resource.SessionRef_builder{Id: v.GetId()}.Build())
	if err != nil {
		return nil, err
	}
	if v.GetStatus().GetRunId() != r.GetRunId() {
		if r.GetRunId() == "" {
			switch v.GetStatus().GetState() {
			case "idle", "working", "waiting_input":
				return v, nil
			}
		}
		return nil, status.Error(codes.FailedPrecondition, "stale run_id")
	}
	first := awaitingFirstRun(v)
	var out *api.Session
	if first {
		state := proto.CloneOf(v.GetStatus())
		state.SetState("starting")
		var pending *resource.Session
		pending, err = s.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef(v.GetRuntimeId()), Status: state, DateUpdatedForce: ptr(true)}.Build())
		if err != nil {
			return nil, err
		}
		defer func() {
			if result != nil {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancel()
				state.SetState("failed")
				_, _ = s.Next().Session().Patch(cleanup, resource.SessionPatchRequest_builder{Ref: sessionRef(v.GetRuntimeId()), Status: state, DateUpdated: pending.GetDateUpdated()}.Build())
			}
		}()
		account, e := s.accountProfile(ctx, v.GetAccount())
		if e != nil {
			return nil, e
		}
		out, err = s.shared.runtime.Create(ctx, &api.CreateRequest{Workspace: v.GetProject().GetWorkspace(), Title: v.GetName(), Agent: v.GetAgent(), Model: v.GetModel(), ClientId: v.GetClientId(), Account: account, AuthBackend: v.GetAccount().GetAuthBackend(), AuthBinding: v.GetAuthBinding().GetBindingId()})
		if err != nil {
			return nil, err
		}
		if out.Id != v.GetRuntimeId() {
			return nil, status.Error(codes.FailedPrecondition, "runtime returned a different session identity")
		}
		// A previous attempt may have persisted its manifest before disconnecting.
		if out.State == "stopped" || out.State == "failed" || out.State == "interrupted" {
			out, err = s.shared.runtime.Resume(ctx, &api.Control{SessionId: out.Id, RunId: out.RunId, ClientId: r.GetClientId()})
		}
	} else {
		out, err = s.shared.runtime.Resume(ctx, &api.Control{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId()})
	}
	if err != nil {
		return nil, err
	}
	out.ProjectId = projectid.String(v.GetProject().GetId())
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	return s.saveSession(ctx, out, "")
}
