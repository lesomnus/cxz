package workspace

import (
	"context"

	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/dockerx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (m *Manager) Paths(ctx context.Context, project, path string, emit func(containerterm.PathListing)) (containerterm.PathListing, error) {
	p, err := m.resolve(ctx, project)
	if err != nil {
		return containerterm.PathListing{}, err
	}
	if p.ContainerID == "" || p.RemoteUser == "" {
		return containerterm.PathListing{}, status.Error(codes.FailedPrecondition, "project container unavailable; start the project")
	}
	c, err := dockerx.Owned(ctx, p.ContainerID, m.Owner, p.ID)
	if err != nil {
		return containerterm.PathListing{}, err
	}
	if !c.State.Running {
		return containerterm.PathListing{}, status.Error(codes.FailedPrecondition, "project is stopped; start the project")
	}
	// Helpers are reused across requests and closed with the manager. Cancellation
	// drains the current request; a directory deadline discards its helper.
	return m.paths.Paths(context.Background(), ctx, projectView(p), path, emit)
}
