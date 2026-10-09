package workspace

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A secret file is written by the workspace helper inside the project
// container, which takes a Docker engine the writer can reach. A client on the
// daemon host does that itself and never puts the secret on a network; a client
// that reached the daemon from elsewhere cannot, so the manager does it on its
// behalf. The client decides whether its link may carry the secret at all --
// the exposed TCP surface and a local socket are indistinguishable here.
func (m *Manager) PutSecretFile(ctx context.Context, in *api.PutSecretFileInput) (*api.SecretFileReply, error) {
	// The secret is cleared as soon as the helper has it, rather than left for
	// the collector to get to.
	defer clear(in.Secret)
	p, err := m.secretProject(ctx, in.Project)
	if err != nil {
		return nil, err
	}
	// Helpers outlive one request and are closed with the manager, the way path
	// completion uses them.
	path, err := m.paths.PutSecret(context.Background(), ctx, projectView(p), in.Session, in.Secret)
	if err != nil {
		return nil, err
	}
	return &api.SecretFileReply{Path: path}, nil
}

func (m *Manager) DeleteSecretFile(ctx context.Context, in *api.DeleteSecretFileInput) (*api.SecretFileReply, error) {
	p, err := m.secretProject(ctx, in.Project)
	if err != nil {
		return nil, err
	}
	return &api.SecretFileReply{}, m.paths.DeleteSecret(context.Background(), ctx, projectView(p), in.Path)
}

// secretProject answers with the running project the helper will write in, or
// says why it cannot: a stopped container has no tmpfs to put a secret in.
func (m *Manager) secretProject(ctx context.Context, project string) (*Project, error) {
	p, err := m.resolve(ctx, project)
	if err != nil {
		return nil, err
	}
	if p.ContainerID == "" || p.RemoteUser == "" {
		return nil, status.Error(codes.FailedPrecondition, "project container unavailable; start the project")
	}
	c, err := dockerx.Owned(ctx, p.ContainerID, m.Owner, p.ID)
	if err != nil {
		return nil, err
	}
	if !c.State.Running {
		return nil, status.Error(codes.FailedPrecondition, "project is stopped; start the project")
	}
	return p, nil
}
