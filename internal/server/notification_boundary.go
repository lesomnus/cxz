package server

import (
	"context"
	"github.com/lesomnus/cxz/api"
)

func (s *Server) NotificationBoundary(ctx context.Context, id string) (uint64, error) {
	if s.manager != nil {
		return s.manager.NotificationBoundary(ctx, id)
	}
	m, err := s.manifest(ctx, id)
	if err != nil {
		return 0, err
	}
	if m.Fork != nil {
		return m.Fork.ThroughSeq + 1, nil
	}
	return 0, nil
}

func (s *Server) NotificationHead(ctx context.Context, id string) (uint64, error) {
	if s.manager != nil {
		return s.manager.NotificationHead(ctx, id)
	}
	v, err := s.Get(ctx, &api.SessionRef{Id: id})
	if err != nil {
		return 0, err
	}
	return v.LastSeq, nil
}
