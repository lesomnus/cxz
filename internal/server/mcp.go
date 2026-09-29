package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"github.com/lesomnus/cxz/internal/mcpruntime"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) mcpState(ctx context.Context) (*api.Receipt, error) {
	desired, e := mcpconfig.LoadRuntime(s.root)
	if e != nil {
		return nil, e
	}
	sessions, e := s.list(ctx)
	if e != nil {
		return nil, e
	}
	out := []mcpconfig.SessionStatus{}
	for _, session := range sessions {
		state, e := s.snapshot(ctx, session)
		if e != nil {
			return nil, e
		}
		if state.State == "stopped" || state.State == "failed" {
			continue
		}
		launch, e := mcpruntime.ReadLaunch(s.root, session.ID)
		v := mcpconfig.SessionStatus{ID: session.ID, Title: session.Title, Pending: e != nil || launch.Config.Digest() != desired.Digest(), Servers: map[string]string{}}
		for id, def := range launch.Config.Servers {
			status := "not connected"
			if def.Kind == "http" {
				status = "agent-managed HTTP"
			} else if b, e := os.ReadFile(mcpruntime.StatePath(s.root, session.ID, id)); e == nil {
				var st mcpruntime.State
				if json.Unmarshal(b, &st) == nil {
					status = st.State
				}
			}
			v.Servers[id] = status
		}
		out = append(out, v)
	}
	b, e := json.Marshal(out)
	return &api.Receipt{Status: string(b)}, e
}

func (s *Server) mcpControl(ctx context.Context, b []byte) (*api.Receipt, error) {
	var r mcpconfig.Request
	if e := json.Unmarshal(b, &r); e != nil {
		return nil, e
	}
	if e := mcpconfig.ValidateID(r.ID); e != nil {
		return nil, e
	}
	session, e := s.manifest(ctx, r.Session)
	if e != nil {
		return nil, e
	}
	if session.ProjectID != r.Project {
		return nil, fmt.Errorf("MCP session belongs to another project")
	}
	launch, e := mcpruntime.ReadLaunch(s.root, r.Session)
	if e != nil {
		return nil, e
	}
	def, ok := launch.Config.Servers[r.ID]
	if !ok {
		return nil, fmt.Errorf("MCP is not enabled in this agent launch")
	}
	out := mcpconfig.Reply{}
	switch r.Action {
	case "logs":
		if def.Kind != "stdio" {
			return nil, fmt.Errorf("stderr logs are available for local external MCPs")
		}
		b, e := os.ReadFile(filepath.Join(core.Dir(s.root, r.Session), "mcp-"+r.ID+".stderr.log"))
		if e != nil && !os.IsNotExist(e) {
			return nil, e
		}
		text := string(b)
		for _, v := range def.Env {
			if v != "" {
				text = strings.ReplaceAll(text, v, "[redacted]")
			}
		}
		out.Log = text
		out.Message = "MCP stderr (up to 1 MiB, reset on each connection)"
	case "restart":
		if def.Kind == "http" {
			return nil, fmt.Errorf("HTTP connection lifecycle is managed by the agent")
		}
		if s.mcpRuntime == nil {
			return nil, fmt.Errorf("MCP runtime unavailable")
		}
		s.mcpRuntime.Disconnect(r.Session, r.ID)
		out.Message = "MCP connection closed; the next request reconnects. In-flight calls are not replayed. Agent launch settings are preserved."
	default:
		return nil, fmt.Errorf("unknown MCP control action")
	}
	b, e = json.Marshal(out)
	return &api.Receipt{Status: string(b)}, e
}
