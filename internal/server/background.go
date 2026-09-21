package server

import (
	"context"
	"encoding/json"

	"github.com/lesomnus/cxz/api"
)

func (s *Server) Background(ctx context.Context, r *api.SessionRef) (*api.BackgroundReply, error) {
	if s.manager != nil {
		return s.manager.Background(ctx, r)
	}
	m, err := s.manifest(ctx, r.Id)
	if err != nil {
		return nil, err
	}
	p, err := s.lockProjection(ctx, m)
	if err != nil {
		return nil, err
	}
	defer p.mu.Unlock()
	data, err := json.Marshal(p.background)
	return &api.BackgroundReply{LastSeq: p.cursor.Seq, Data: data}, err
}
