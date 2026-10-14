package workspace

import (
	"context"
	"github.com/lesomnus/cxz/internal/accounts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (m *Manager) AccountStatus(ctx context.Context, account, agent, backend string) (accounts.AuthStatus, error) {
	if err := ctx.Err(); err != nil {
		return accounts.AuthStatus{}, err
	}
	if _, err := accounts.Resolve(agent, backend); err != nil {
		return accounts.AuthStatus{}, err
	}
	if backend == accounts.ProjectLocalOAuth {
		return accounts.AuthStatus{State: "session_scoped", Scope: "session", Message: "Login belongs to each session. Manage it from that session's details."}, nil
	}
	if m.Owner == "" {
		return accounts.AuthStatus{}, status.Error(codes.FailedPrecondition, "central authentication requires Manager")
	}
	return accounts.CentralStatus(m.Root, account)
}

func (m *Manager) LogoutAccount(ctx context.Context, account, agent, backend string) error {
	if agent != "codex" || backend != accounts.BrokeredAccessToken {
		return status.Error(codes.FailedPrecondition, "this account uses independent session logins")
	}
	if m.Owner == "" {
		return status.Error(codes.FailedPrecondition, "central authentication requires Manager")
	}
	if err := accounts.CentralLogout(ctx, m.Root, account); err != nil {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return nil
}
