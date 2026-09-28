package server

import (
	"context"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) Download(ctx context.Context, project, path string, dst io.Writer) error {
	if s.manager == nil {
		return status.Error(codes.FailedPrecondition, "container downloads require the manager endpoint")
	}
	return s.manager.Download(ctx, project, path, dst)
}
