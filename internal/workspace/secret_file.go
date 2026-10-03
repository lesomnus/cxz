package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/secretfile"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A secret file is written by the workspace helper inside the project
// container, which takes a Docker engine the writer can reach. A client on the
// daemon host does that itself and never puts the secret on a network; a client
// that reached the daemon from elsewhere cannot, so the manager does it on its
// behalf. The client decides whether its link may carry the secret at all --
// the exposed TCP surface and a local socket are indistinguishable here.
func (m *Manager) secretFile(ctx context.Context, spec []byte) (*api.Receipt, error) {
	var r secretfile.Request
	if err := json.Unmarshal(spec, &r); err != nil {
		return nil, err
	}
	defer clear(r.Secret)
	p, err := m.resolve(ctx, r.Project)
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
	// Helpers outlive one request and are closed with the manager, the way path
	// completion uses them.
	switch r.Action {
	case "put":
		path, err := m.paths.PutSecret(context.Background(), ctx, projectView(p), r.Session, r.Secret)
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(secretfile.Reply{Path: path})
		if err != nil {
			return nil, err
		}
		return &api.Receipt{Status: string(b)}, nil
	case "delete":
		if err := m.paths.DeleteSecret(context.Background(), ctx, projectView(p), r.Path); err != nil {
			return nil, err
		}
		return &api.Receipt{Status: "{}"}, nil
	}
	return nil, fmt.Errorf("unknown secret action %q", r.Action)
}
