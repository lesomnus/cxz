package server

import (
	"context"

	"github.com/lesomnus/cxz/internal/containerterm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) Paths(ctx context.Context, project, path string, emit func(containerterm.PathListing)) (containerterm.PathListing, error) {
	if s.manager == nil {
		return containerterm.PathListing{}, status.Error(codes.FailedPrecondition, "container path browsing requires the manager endpoint")
	}
	return s.manager.Paths(ctx, project, path, emit)
}
