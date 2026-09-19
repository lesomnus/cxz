package server

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

func (s *Server) Permission(ctx context.Context, r *api.PermissionInput) (*api.Receipt, error) {
	if s.manager != nil {
		conn, client, err := s.manager.ClientFor(ctx, r.SessionId)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		return client.Permission(ctx, r)
	}
	return s.command(ctx, r.SessionId, "permission", core.Command{RunID: r.RunId, ClientID: r.ClientId, Text: r.Mode})
}
