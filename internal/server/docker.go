package server

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/filemap"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"github.com/lesomnus/cxz/internal/skillconfig"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) Docker(ctx context.Context, r *api.DockerInput) (*api.Receipt, error) {
	if s.manager == nil && r.Action == "mcp-control" {
		return s.mcpControl(ctx, r.Spec)
	}
	if s.manager == nil && r.Action == "mcp-state" {
		return s.mcpState(ctx)
	}
	if s.manager == nil && r.Action == "skills-sync" {
		b, err := filemap.Decode(r.Spec)
		if err != nil {
			return nil, err
		}
		err = skillconfig.SaveRuntime(s.root, b)
		return &api.Receipt{Status: "Skills saved for new agent launches"}, err
	}
	if s.manager == nil && r.Action == "mcp-sync" {
		var v mcpconfig.Snapshot
		if err := json.Unmarshal(r.Spec, &v); err != nil {
			return nil, err
		}
		err := mcpconfig.SaveRuntime(s.root, v)
		return &api.Receipt{Status: "MCP configuration saved for new agent launches"}, err
	}
	if s.manager == nil {
		return nil, status.Error(codes.FailedPrecondition, "managed Docker requires an installed host manager")
	}
	return s.manager.Docker(ctx, r)
}
