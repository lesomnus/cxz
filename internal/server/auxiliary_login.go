package server

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/auxiliary"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Auxiliary authentication belongs to the Manager's dedicated account profile.
// The lifecycle stack uses Server as its runtime, not workspace.Manager directly.
func (s *Server) AuxiliaryLogin(ctx context.Context, p auxiliary.Profile, input io.Reader, output, errOutput io.Writer) error {
	if s.manager == nil {
		return status.Error(codes.FailedPrecondition, "auxiliary login requires the manager endpoint")
	}
	return s.manager.AuxiliaryLogin(ctx, p, input, output, errOutput)
}
