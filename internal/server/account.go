package server

import (
	"context"
	"github.com/lesomnus/cxz/internal/accounts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) AccountStatus(ctx context.Context, alias, agent, backend string) (accounts.AuthStatus, error) {
	if s.manager == nil {
		return accounts.AuthStatus{}, status.Error(codes.FailedPrecondition, "account authentication requires the Manager endpoint")
	}
	return s.manager.AccountStatus(ctx, alias, agent, backend)
}
func (s *Server) LogoutAccount(ctx context.Context, alias, agent, backend string) error {
	if s.manager == nil {
		return status.Error(codes.FailedPrecondition, "account logout requires the Manager endpoint")
	}
	return s.manager.LogoutAccount(ctx, alias, agent, backend)
}
func (s *Server) AccountEraseAllowed(ctx context.Context) error {
	if s.manager == nil {
		return status.Error(codes.FailedPrecondition, "account erasure requires the Manager endpoint")
	}
	return s.requireManagerAuthority(ctx, "erase accounts from the host client, not from inside a project")
}
func (s *Server) EraseAccount(ctx context.Context, alias, agent, backend string) error {
	if err := s.AccountEraseAllowed(ctx); err != nil {
		return err
	}
	return s.manager.EraseAccount(ctx, alias, agent, backend)
}
