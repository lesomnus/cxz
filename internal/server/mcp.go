package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"github.com/lesomnus/cxz/internal/mcpruntime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The registrations are the installation's, so they are answered by the
// manager. A project runtime answers the three that are about this container:
// what it was told to use, what its sessions actually launched with, and one
// session's connection to one of them.

func (s *Server) requireManagerMCP() error {
	if s.manager == nil {
		return status.Error(codes.FailedPrecondition, "MCP registrations belong to an installed host manager")
	}
	return nil
}

func (s *Server) GetMcpServers(ctx context.Context, r *api.McpServersInput) (*api.McpServersReply, error) {
	if err := s.requireManagerMCP(); err != nil {
		return nil, err
	}
	return s.manager.GetMcpServers(ctx, r)
}

func (s *Server) PutMcpServer(ctx context.Context, r *api.PutMcpServerInput) (*api.McpServersReply, error) {
	if err := s.requireManagerMCP(); err != nil {
		return nil, err
	}
	return s.manager.PutMcpServer(ctx, r)
}

func (s *Server) RemoveMcpServer(ctx context.Context, r *api.McpServerInput) (*api.McpServersReply, error) {
	if err := s.requireManagerMCP(); err != nil {
		return nil, err
	}
	return s.manager.RemoveMcpServer(ctx, r)
}

func (s *Server) SetMcpServerDefault(ctx context.Context, r *api.McpServerDefaultInput) (*api.McpServersReply, error) {
	if err := s.requireManagerMCP(); err != nil {
		return nil, err
	}
	return s.manager.SetMcpServerDefault(ctx, r)
}

func (s *Server) SetProjectMcpServer(ctx context.Context, r *api.ProjectMcpServerInput) (*api.McpServersReply, error) {
	if err := s.requireManagerMCP(); err != nil {
		return nil, err
	}
	return s.manager.SetProjectMcpServer(ctx, r)
}

func (s *Server) ClearProjectMcpServer(ctx context.Context, r *api.ClearProjectMcpServerInput) (*api.McpServersReply, error) {
	if err := s.requireManagerMCP(); err != nil {
		return nil, err
	}
	return s.manager.ClearProjectMcpServer(ctx, r)
}

// SyncMcpServers stores what the manager resolved for this project, for the
// next agent launch to use. A manager has nothing to store: it is the caller.
func (s *Server) SyncMcpServers(ctx context.Context, r *api.SyncMcpServersInput) (*api.Receipt, error) {
	if s.manager != nil {
		return s.manager.SyncMcpServers(ctx, r)
	}
	snapshot := mcpconfig.Snapshot{Servers: map[string]mcpconfig.Server{}}
	for id, v := range r.Servers {
		snapshot.Servers[id] = mcpconfig.Server{
			Name: v.Name, Kind: v.Kind, Enabled: v.Enabled,
			Command: v.Command, Args: v.Args, Env: v.Env,
			URL: v.Url, Headers: v.Headers,
		}
	}
	err := mcpconfig.SaveRuntime(s.root, snapshot)
	return &api.Receipt{Status: "MCP configuration saved for new agent launches"}, err
}

// McpSessions reports what each live session launched with. pending here is
// measured against what this project was last told; the manager measures it
// again against what it holds now, because the two can differ.
func (s *Server) McpSessions(ctx context.Context, r *api.McpSessionsInput) (*api.McpSessionsReply, error) {
	if s.manager != nil {
		return s.manager.McpSessions(ctx, r)
	}
	desired, e := mcpconfig.LoadRuntime(s.root)
	if e != nil {
		return nil, e
	}
	sessions, e := s.list(ctx)
	if e != nil {
		return nil, e
	}
	out := &api.McpSessionsReply{}
	for _, session := range sessions {
		state, e := s.snapshot(ctx, session)
		if e != nil {
			return nil, e
		}
		if state.State == "stopped" || state.State == "failed" {
			continue
		}
		launch, e := mcpruntime.ReadLaunch(s.root, session.ID)
		v := &api.McpSessionStatus{
			SessionId: session.ID, Title: session.Title,
			Pending: e != nil || launch.Config.Digest() != desired.Digest(),
			Servers: map[string]string{},
		}
		if e == nil {
			v.LaunchDigest = launch.Config.Digest()
		}
		for id, def := range launch.Config.Servers {
			v.Servers[id] = s.mcpConnectionState(session.ID, id, def)
		}
		out.Sessions = append(out.Sessions, v)
	}
	return out, nil
}

func (s *Server) mcpConnectionState(session, id string, def mcpconfig.Server) string {
	if def.Kind == "http" {
		return "agent-managed HTTP"
	}
	b, e := os.ReadFile(mcpruntime.StatePath(s.root, session, id))
	if e != nil {
		return "not connected"
	}
	var st mcpruntime.State
	if json.Unmarshal(b, &st) != nil {
		return "not connected"
	}
	return st.State
}

func (s *Server) McpLogs(ctx context.Context, r *api.McpLogsInput) (*api.McpLogsReply, error) {
	if s.manager != nil {
		return s.manager.McpLogs(ctx, r)
	}
	def, err := s.mcpLaunched(ctx, r.SessionId, r.Id)
	if err != nil {
		return nil, err
	}
	if def.Kind != "stdio" {
		return nil, fmt.Errorf("stderr logs are available for local external MCPs")
	}
	b, err := os.ReadFile(filepath.Join(core.Dir(s.root, r.SessionId), "mcp-"+r.Id+".stderr.log"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	// A server that prints its own configuration on startup would otherwise
	// hand back the secret it was given.
	text := string(b)
	for _, v := range def.Env {
		if v != "" {
			text = strings.ReplaceAll(text, v, "[redacted]")
		}
	}
	return &api.McpLogsReply{
		Text:    text,
		Message: "MCP stderr (up to 1 MiB, reset on each connection)",
	}, nil
}

func (s *Server) RestartMcp(ctx context.Context, r *api.RestartMcpInput) (*api.Receipt, error) {
	if s.manager != nil {
		return s.manager.RestartMcp(ctx, r)
	}
	def, err := s.mcpLaunched(ctx, r.SessionId, r.Id)
	if err != nil {
		return nil, err
	}
	if def.Kind == "http" {
		return nil, fmt.Errorf("HTTP connection lifecycle is managed by the agent")
	}
	if s.mcpRuntime == nil {
		return nil, fmt.Errorf("MCP runtime unavailable")
	}
	s.mcpRuntime.Disconnect(r.SessionId, r.Id)
	return &api.Receipt{Status: "MCP connection closed; the next request reconnects. In-flight calls are not replayed. Agent launch settings are preserved."}, nil
}

// mcpLaunched finds the definition this session actually started with, which is
// what both of these operate on. A registration enabled now but not at launch
// has no connection here to read from or to reconnect.
func (s *Server) mcpLaunched(ctx context.Context, session, id string) (mcpconfig.Server, error) {
	if err := mcpconfig.ValidateID(id); err != nil {
		return mcpconfig.Server{}, err
	}
	if _, err := s.manifest(ctx, session); err != nil {
		return mcpconfig.Server{}, err
	}
	launch, err := mcpruntime.ReadLaunch(s.root, session)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	def, ok := launch.Config.Servers[id]
	if !ok {
		return mcpconfig.Server{}, fmt.Errorf("MCP is not enabled in this agent launch")
	}
	return def, nil
}
