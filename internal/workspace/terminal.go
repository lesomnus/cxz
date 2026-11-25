package workspace

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (m *Manager) OpenTerminal(ctx context.Context, project string, columns, rows int) (containerterm.Terminal, error) {
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := m.resolve(check, project)
	if err != nil {
		return nil, err
	}
	if p.ContainerID == "" || p.RemoteUser == "" || p.RemoteWorkspace == "" {
		return nil, status.Error(codes.FailedPrecondition, "project container unavailable; start the project")
	}
	if m.kube != nil {
		// The same rule as a Docker project's terminal: not for remote callers.
		if err = transport.LocalOnly(ctx, "container terminal"); err != nil {
			return nil, err
		}
		if _, err = m.kubeAddress(check, p); err != nil {
			return nil, status.Error(codes.FailedPrecondition, "project is stopped; start the project")
		}
		args := append([]string{"-it", "--user", p.RemoteUser, "--workdir", p.RemoteWorkspace, "--env", "TERM=xterm-256color", p.ContainerID}, containerterm.LoginShell()...)
		return containerterm.StartPTY(m.containerCommand(ctx, args...), columns, rows)
	}
	c, err := dockerx.Owned(check, p.ContainerID, m.Owner, p.storageID())
	if err != nil {
		return nil, err
	}
	if !c.State.Running {
		return nil, status.Error(codes.FailedPrecondition, "project is stopped; start the project")
	}
	view := projectView(p)
	view.ContainerId = c.ID
	return containerterm.OpenPTY(ctx, view, columns, rows)
}
