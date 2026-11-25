package server

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) BuildProject(ctx context.Context, r *api.BuildProjectInput) (*api.Receipt, error) {
	if s.manager == nil {
		return nil, status.Error(codes.PermissionDenied, "project lifecycle is not available in this runtime")
	}
	return s.manager.BuildProject(ctx, r)
}
