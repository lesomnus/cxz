package server

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/filemap"
)

func (s *Server) FileMappings(ctx context.Context, r *api.FileMappingsInput) (*api.Receipt, error) {
	b, err := filemap.Decode(r.Bundle)
	if err != nil {
		return nil, err
	}
	if s.manager != nil {
		return s.manager.FileMappings(ctx, b)
	}
	if err := filemap.Save(s.root, b); err != nil {
		return nil, err
	}
	return &api.Receipt{Status: "saved; applies on next agent start"}, nil
}
