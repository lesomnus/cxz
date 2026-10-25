package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/internal/projectid"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s SessionServer) Fork(ctx context.Context, r *resource.SessionControl) (*resource.Session, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	if r.GetClientId() == "" || r.GetRunId() == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id and client_id required")
	}
	source, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	child, err := s.shared.runtime.ForkSession(ctx, &api.SessionForkInput{SessionId: source.GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId()})
	if err != nil {
		return nil, err
	}
	if !projectid.Same(child.ProjectId, projectid.String(source.GetProject().GetId())) {
		return nil, status.Error(codes.FailedPrecondition, "fork project mismatch")
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	return s.saveSession(ctx, child, r.GetClientId())
}
