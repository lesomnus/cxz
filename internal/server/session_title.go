package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SetSessionTitle is a local lifecycle hook, not an auxiliary execution RPC.
func (s *Server) SetSessionTitle(ctx context.Context, id, text string) (string, error) {
	if s.manager == nil {
		return "", status.Error(codes.FailedPrecondition, "session title updates require an installed host manager")
	}
	return s.manager.SetSessionTitle(ctx, id, text)
}
