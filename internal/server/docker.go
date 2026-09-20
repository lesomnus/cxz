package server

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) Docker(ctx context.Context, r *api.DockerInput) (*api.Receipt, error) {
	if s.manager == nil {
		return nil, status.Error(codes.FailedPrecondition, "managed Docker requires an installed host manager")
	}
	return s.manager.Docker(ctx, r)
}
