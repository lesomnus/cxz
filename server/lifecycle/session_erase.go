package lifecycle

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Runtime cleanup finishes before the durable record disappears, so failed
// cleanup leaves the session available for an explicit retry.
func (s SessionServer) Erase(ctx context.Context, ref *resource.SessionRef) (*resource.SessionEraseResponse, error) {
	if ref == nil || (len(ref.GetId()) == 0 && ref.GetAlias() == "" && ref.GetRuntimeId() == "") {
		return nil, status.Error(codes.InvalidArgument, "name the session to erase")
	}
	if err := s.effect(); err != nil {
		return nil, err
	}
	s.shared.transition.Lock()
	defer s.shared.transition.Unlock()
	if err := s.sync(ctx); err != nil {
		return nil, err
	}
	return s.eraseSession(ctx, ref)
}

// Caller holds transition so account erasure can reuse the same purge without
// admitting new sessions between the cascade and deletion of the account.
func (s SessionServer) eraseSession(ctx context.Context, ref *resource.SessionRef) (*resource.SessionEraseResponse, error) {
	v, err := s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
	if status.Code(err) == codes.NotFound {
		return &resource.SessionEraseResponse{}, nil
	}
	if err != nil {
		return nil, err
	}
	switch v.GetStatus().GetState() {
	case "idle", "working", "running", "waiting", "waiting_input", "starting":
		stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if _, err = s.shared.runtime.Stop(stopCtx, &api.Control{SessionId: v.GetRuntimeId(), RunId: v.GetStatus().GetRunId(), ClientId: core.ID()}); err != nil {
			return nil, err
		}
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			current, e := s.shared.runtime.Get(stopCtx, &api.SessionRef{Id: v.GetRuntimeId()})
			if e != nil {
				return nil, e
			}
			if current.State == "stopped" || current.State == "failed" {
				break
			}
			select {
			case <-stopCtx.Done():
				return nil, stopCtx.Err()
			case <-ticker.C:
			}
		}
	}
	forgetAuxiliary(ctx, s.shared.runtime, v.GetRuntimeId())
	if _, err = s.shared.runtime.EraseSession(ctx, &api.SessionEraseInput{SessionId: v.GetRuntimeId(), ProjectId: projectid.String(v.GetProject().GetId()), CreateId: v.GetClientId()}); err != nil {
		return nil, err
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	canonical := resource.SessionRef_builder{Id: v.GetId()}.Build()
	if _, err = s.Next().Session().Erase(ctx, canonical); err != nil {
		return nil, err
	}
	if err = s.purgeSessionAttention(ctx, v.GetId()); err != nil {
		return nil, err
	}
	tx, err := s.shared.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM audit WHERE object_id=?", projectid.String(v.GetId())); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM session WHERE id=?", projectid.String(v.GetId())); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return resource.SessionEraseResponse_builder{Erased: ptr(true)}.Build(), nil
}
