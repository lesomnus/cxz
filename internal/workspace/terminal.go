package workspace

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/dockerx"
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
	c, err := dockerx.Owned(check, p.ContainerID, m.Owner, p.ID)
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
