package server

import (
	"context"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/internal/workspace"
)

func (s *Server) LegacyProjectID(ctx context.Context, id string) (string, error) {
	if s.manager != nil {
		return s.manager.LegacyProjectID(ctx, id)
	}
	r, err := workspace.LoadRuntime(s.root)
	if err != nil {
		return "", err
	}
	if projectid.Legacy(r.ProjectID) && projectid.Same(r.ProjectID, id) {
		return r.ProjectID, nil
	}
	if projectid.Legacy(r.LegacyProjectID) && projectid.Same(r.LegacyProjectID, id) {
		return r.LegacyProjectID, nil
	}
	return "", nil
}
