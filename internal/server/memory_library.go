package server

import (
	"context"
	"github.com/lesomnus/cxz/internal/memorylib"
)

func (s *Server) Library(ctx context.Context, id string, q memorylib.Request) (memorylib.Reply, error) {
	if s.manager != nil {
		return s.manager.Library(ctx, id, q)
	}
	session, e := s.manifest(ctx, id)
	if e != nil {
		return memorylib.Reply{}, e
	}
	return memorylib.New(s.root, session).Do(ctx, q)
}
