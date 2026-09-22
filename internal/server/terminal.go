package server

import (
	"context"

	"github.com/lesomnus/cxz/internal/containerterm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) OpenTerminal(ctx context.Context, project string, columns, rows int) (containerterm.Terminal, error) {
	if s.manager == nil {
		return nil, status.Error(codes.FailedPrecondition, "container terminal requires the manager endpoint")
	}
	return s.manager.OpenTerminal(ctx, project, columns, rows)
}
