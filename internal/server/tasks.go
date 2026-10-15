package server

import (
	"context"
	"google.golang.org/grpc"
)

func (s *Server) TaskConnection(ctx context.Context, project string) (*grpc.ClientConn, error) {
	if s.manager == nil {
		return nil, nil
	}
	return s.manager.ClientForProject(ctx, project)
}
