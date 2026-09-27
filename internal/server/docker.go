package server

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) Docker(ctx context.Context, r *api.DockerInput) (*api.Receipt, error) {
	if s.manager == nil && r.Action == "history-floor" {
		var ref api.SessionRef
		if err := json.Unmarshal(r.Spec, &ref); err != nil {
			return nil, err
		}
		manifest, err := s.manifest(ctx, ref.Id)
		if err != nil {
			return nil, err
		}
		p, err := s.lockProjection(ctx, manifest)
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(core.HistoryBoundary{Through: p.floor})
		p.mu.Unlock()
		return &api.Receipt{Status: string(b)}, err
	}
	if s.manager == nil && r.Action == "history-checkpoint-ready" {
		_, err := s.db.ExecContext(ctx, "PRAGMA user_version=2")
		return &api.Receipt{Status: "ready"}, err
	}
	if s.manager == nil && r.Action == "history-policy" {
		if len(r.Spec) > 0 {
			var p historypolicy.Policy
			if err := json.Unmarshal(r.Spec, &p); err != nil {
				return nil, err
			}
			if err := historypolicy.Save(s.root, p); err != nil {
				return nil, err
			}
		}
		p, err := historypolicy.Load(s.root)
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(p)
		return &api.Receipt{Status: string(b)}, err
	}
	if s.manager == nil {
		return nil, status.Error(codes.FailedPrecondition, "managed Docker requires an installed host manager")
	}
	return s.manager.Docker(ctx, r)
}
