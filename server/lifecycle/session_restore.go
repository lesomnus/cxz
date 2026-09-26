package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Restoring relists the same durable identity. The journal, provider thread,
// account, permissions and attachments stay in place; no new session is made.
func (s SessionServer) Restore(ctx context.Context, ref *resource.SessionRef) (*resource.Session, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	s.shared.transition.Lock()
	defer s.shared.transition.Unlock()
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	v, err := s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	if !v.GetProject().GetListed() {
		return nil, status.Error(codes.FailedPrecondition, "restore the project before its sessions")
	}
	if v.GetListed() {
		return v, nil
	}
	alias, err := s.newSessionAlias(ctx)
	if err != nil {
		return nil, err
	}
	st := v.GetStatus()
	if st == nil {
		st = &resource.SessionStatus{}
	}
	st.SetState("stopped")
	st.SetPending(nil)
	return s.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: ref, Listed: ptr(true), Alias: &alias, Status: st, DateUpdatedForce: ptr(true)}.Build())
}
