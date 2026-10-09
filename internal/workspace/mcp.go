package workspace

import (
	"context"
	"fmt"
	"strings"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/mcpconfig"
)

func (m *Manager) syncMCP(ctx context.Context, client api.SessionsClient, project string) error {
	c, e := mcpconfig.Load(m.Root)
	if e != nil {
		return e
	}
	_, e = client.SyncMcpServers(ctx, &api.SyncMcpServersInput{Servers: mcpServers(c.Resolve(project).Servers)})
	return e
}

func (m *Manager) GetMcpServers(ctx context.Context, r *api.McpServersInput) (*api.McpServersReply, error) {
	project, e := m.projectScope(ctx, r.Project)
	if e != nil {
		return nil, e
	}
	listing, e := mcpconfig.List(m.Root, project)
	if e != nil {
		return nil, e
	}
	return mcpListing(listing), nil
}

func (m *Manager) PutMcpServer(ctx context.Context, r *api.PutMcpServerInput) (*api.McpServersReply, error) {
	return m.changeMCP(ctx, "", func(string) (mcpconfig.Listing, error) {
		return mcpconfig.Put(m.Root, r.Id, mcpServer(r.Server))
	})
}

func (m *Manager) RemoveMcpServer(ctx context.Context, r *api.McpServerInput) (*api.McpServersReply, error) {
	return m.changeMCP(ctx, "", func(string) (mcpconfig.Listing, error) {
		return mcpconfig.Remove(m.Root, r.Id)
	})
}

func (m *Manager) SetMcpServerDefault(ctx context.Context, r *api.McpServerDefaultInput) (*api.McpServersReply, error) {
	return m.changeMCP(ctx, "", func(string) (mcpconfig.Listing, error) {
		return mcpconfig.SetDefault(m.Root, r.Id, r.Enabled)
	})
}

func (m *Manager) SetProjectMcpServer(ctx context.Context, r *api.ProjectMcpServerInput) (*api.McpServersReply, error) {
	return m.changeMCP(ctx, r.Project, func(project string) (mcpconfig.Listing, error) {
		return mcpconfig.SetProject(m.Root, project, r.Id, r.Enabled)
	})
}

func (m *Manager) ClearProjectMcpServer(ctx context.Context, r *api.ClearProjectMcpServerInput) (*api.McpServersReply, error) {
	return m.changeMCP(ctx, r.Project, func(project string) (mcpconfig.Listing, error) {
		return mcpconfig.ClearProject(m.Root, project, r.Id)
	})
}

// McpSessions asks the project container what its live sessions launched with,
// then decides pending against what this manager holds now -- the project only
// knows what it was last told, which may itself be behind.
func (m *Manager) McpSessions(ctx context.Context, r *api.McpSessionsInput) (*api.McpSessionsReply, error) {
	p, e := m.resolve(ctx, r.Project)
	if e != nil {
		return nil, e
	}
	if p.ContainerID == "" {
		return &api.McpSessionsReply{Message: "Project is not running, so no session has launched with these settings."}, nil
	}
	conn, client, e := m.client(ctx, p)
	if e != nil {
		return &api.McpSessionsReply{Message: "Runtime status unavailable."}, nil
	}
	out, e := client.McpSessions(ctx, &api.McpSessionsInput{Project: p.ID})
	conn.Close()
	if e != nil {
		return &api.McpSessionsReply{Message: "Runtime status unavailable."}, nil
	}
	cfg, e := mcpconfig.Load(m.Root)
	if e != nil {
		return nil, e
	}
	desired := cfg.Resolve(p.ID).Digest()
	for _, v := range out.Sessions {
		v.Pending = v.LaunchDigest != desired
	}
	return out, nil
}

// SyncMcpServers is answered by a project container, not by a manager: the
// manager is the side that calls it.
func (m *Manager) SyncMcpServers(context.Context, *api.SyncMcpServersInput) (*api.Receipt, error) {
	return nil, fmt.Errorf("MCP settings are delivered to a project, not to a manager")
}

// McpLogs and RestartMcp are one session's, so the manager hands them to the
// project container that holds the session rather than answering itself.
func (m *Manager) McpLogs(ctx context.Context, r *api.McpLogsInput) (*api.McpLogsReply, error) {
	conn, client, e := m.ClientFor(ctx, r.SessionId)
	if e != nil {
		return nil, e
	}
	defer conn.Close()
	return client.McpLogs(ctx, r)
}

func (m *Manager) RestartMcp(ctx context.Context, r *api.RestartMcpInput) (*api.Receipt, error) {
	conn, client, e := m.ClientFor(ctx, r.SessionId)
	if e != nil {
		return nil, e
	}
	defer conn.Close()
	return client.RestartMcp(ctx, r)
}

// changeMCP saves the change and then pushes it, because the change decides
// what a project may launch with; leaving it until something else happens to
// sync would mean a server is switched on and absent.
func (m *Manager) changeMCP(ctx context.Context, handle string, change func(string) (mcpconfig.Listing, error)) (*api.McpServersReply, error) {
	project, e := m.projectScope(ctx, handle)
	if e != nil {
		return nil, e
	}
	listing, e := change(project)
	if e != nil {
		return nil, e
	}
	out := mcpListing(listing)
	if failed := m.pushMCP(ctx, project); len(failed) > 0 {
		out.Message += " Runtime sync pending (retry the setting change or launch through the manager): " + strings.Join(failed, ", ")
	}
	return out, nil
}

func (m *Manager) pushMCP(ctx context.Context, project string) []string {
	projects, e := m.all(ctx)
	if e != nil {
		return []string{e.Error()}
	}
	var failed []string
	for _, p := range projects {
		if project != "" && project != p.ID || p.ContainerID == "" {
			continue
		}
		v, e := dockerx.Owned(ctx, p.ContainerID, m.Owner, p.ID)
		if e == nil && !v.State.Running {
			continue
		}
		if e == nil {
			conn, client, err := m.client(ctx, p)
			e = err
			if e == nil {
				e = m.syncMCP(ctx, client, p.ID)
				conn.Close()
			}
		}
		if e != nil {
			failed = append(failed, p.Name)
		}
	}
	return failed
}

func mcpListing(v mcpconfig.Listing) *api.McpServersReply {
	out := &api.McpServersReply{Message: v.Message}
	for _, e := range v.Entries {
		out.Entries = append(out.Entries, &api.McpEntry{
			Id: e.ID, Server: mcpServerOf(e.Server), Override: e.Override, Effective: e.Effective,
		})
	}
	return out
}

func mcpServerOf(s mcpconfig.Server) *api.McpServer {
	return &api.McpServer{
		Name: s.Name, Kind: s.Kind, Enabled: s.Enabled,
		Command: s.Command, Args: s.Args, Env: s.Env,
		Url: s.URL, Headers: s.Headers,
	}
}

func mcpServer(s *api.McpServer) mcpconfig.Server {
	if s == nil {
		return mcpconfig.Server{}
	}
	return mcpconfig.Server{
		Name: s.Name, Kind: s.Kind, Enabled: s.Enabled,
		Command: s.Command, Args: s.Args, Env: s.Env,
		URL: s.Url, Headers: s.Headers,
	}
}

func mcpServers(in map[string]mcpconfig.Server) map[string]*api.McpServer {
	out := make(map[string]*api.McpServer, len(in))
	for id, s := range in {
		out[id] = mcpServerOf(s)
	}
	return out
}
