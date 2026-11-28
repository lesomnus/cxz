package server

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/workspace"
)

// What a session takes up, told apart by who decided it should grow. The
// journal is cxz's own and a retention policy can reach it; the provider's
// transcripts are the agent's, grow with every turn, and nothing here prunes
// them -- which is the number worth seeing before deciding to start over. The
// rest of the profile is caches and temporary files, interesting mostly when it
// is unexpectedly the large part.
func (s *Server) SessionStorage(ctx context.Context, r *api.SessionRef) (*api.StorageUsage, error) {
	if s.manager != nil {
		return s.manager.SessionStorage(ctx, r)
	}
	session, err := s.manifest(ctx, r.Id)
	if err != nil {
		return nil, err
	}
	return workspace.MeasureSessionStorage(ctx, s.root, session)
}
