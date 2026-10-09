package resourceclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/mcpkind"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

func (c *Client) GetMcpServers(ctx context.Context, r *api.McpServersInput, opts ...grpc.CallOption) (*api.McpServersReply, error) {
	ref, err := c.projectRef(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	return mcpServersReply(c.projects.GetMcpServers(ctx, resource.McpServersRequest_builder{Ref: ref}.Build(), opts...))
}

func (c *Client) PutMcpServer(ctx context.Context, r *api.PutMcpServerInput, opts ...grpc.CallOption) (*api.McpServersReply, error) {
	return mcpServersReply(c.projects.PutMcpServer(ctx, resource.PutMcpServerRequest_builder{
		Id: &r.Id, Server: resourceMcpServer(r.Server),
	}.Build(), opts...))
}

func (c *Client) RemoveMcpServer(ctx context.Context, r *api.McpServerInput, opts ...grpc.CallOption) (*api.McpServersReply, error) {
	return mcpServersReply(c.projects.RemoveMcpServer(ctx, resource.McpServerRequest_builder{Id: &r.Id}.Build(), opts...))
}

func (c *Client) SetMcpServerDefault(ctx context.Context, r *api.McpServerDefaultInput, opts ...grpc.CallOption) (*api.McpServersReply, error) {
	return mcpServersReply(c.projects.SetMcpServerDefault(ctx, resource.McpServerDefaultRequest_builder{
		Id: &r.Id, Enabled: &r.Enabled,
	}.Build(), opts...))
}

func (c *Client) SetProjectMcpServer(ctx context.Context, r *api.ProjectMcpServerInput, opts ...grpc.CallOption) (*api.McpServersReply, error) {
	ref, err := c.projectRef(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	return mcpServersReply(c.projects.SetProjectMcpServer(ctx, resource.ProjectMcpServerRequest_builder{
		Ref: ref, Id: &r.Id, Enabled: &r.Enabled,
	}.Build(), opts...))
}

func (c *Client) ClearProjectMcpServer(ctx context.Context, r *api.ClearProjectMcpServerInput, opts ...grpc.CallOption) (*api.McpServersReply, error) {
	ref, err := c.projectRef(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	return mcpServersReply(c.projects.ClearProjectMcpServer(ctx, resource.ClearProjectMcpServerRequest_builder{
		Ref: ref, Id: &r.Id,
	}.Build(), opts...))
}

func (c *Client) McpSessions(ctx context.Context, r *api.McpSessionsInput, opts ...grpc.CallOption) (*api.McpSessionsReply, error) {
	ref, err := c.projectRef(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	v, err := c.projects.McpSessions(ctx, resource.McpSessionsRequest_builder{Ref: ref}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	out := &api.McpSessionsReply{Message: v.GetMessage()}
	for _, s := range v.GetSessions() {
		out.Sessions = append(out.Sessions, &api.McpSessionStatus{
			SessionId: s.GetSession(), Title: s.GetTitle(), Pending: s.GetPending(),
			LaunchDigest: s.GetLaunchDigest(), Servers: s.GetServers(),
		})
	}
	return out, nil
}

func (c *Client) SyncMcpServers(ctx context.Context, r *api.SyncMcpServersInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	servers := map[string]*resource.McpServer{}
	for id, v := range r.Servers {
		servers[id] = resourceMcpServer(v)
	}
	out, err := c.projects.SyncMcpServers(ctx, resource.SyncMcpServersRequest_builder{Servers: servers}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.Receipt{Status: out.GetStatus()}, nil
}

func (c *Client) McpLogs(ctx context.Context, r *api.McpLogsInput, opts ...grpc.CallOption) (*api.McpLogsReply, error) {
	v, err := c.sessions.McpLogs(ctx, resource.SessionMcpRequest_builder{
		Ref: sr(r.SessionId), Id: &r.Id,
	}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.McpLogsReply{Text: v.GetText(), Message: v.GetMessage()}, nil
}

func (c *Client) RestartMcp(ctx context.Context, r *api.RestartMcpInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	v, err := c.sessions.RestartMcp(ctx, resource.SessionMcpRequest_builder{
		Ref: sr(r.SessionId), Id: &r.Id,
	}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.Receipt{Status: v.GetStatus()}, nil
}

func mcpServersReply(v *resource.McpServersReply, err error) (*api.McpServersReply, error) {
	if err != nil {
		return nil, err
	}
	out := &api.McpServersReply{Message: v.GetMessage()}
	for _, e := range v.GetEntries() {
		entry := &api.McpEntry{
			Id: e.GetId(), Server: runtimeMcpServer(e.GetServer()), Effective: e.GetEffective(),
		}
		if e.HasOverride() {
			override := e.GetOverride()
			entry.Override = &override
		}
		out.Entries = append(out.Entries, entry)
	}
	return out, nil
}

func resourceMcpServer(v *api.McpServer) *resource.McpServer {
	if v == nil {
		return nil
	}
	kind := mcpkind.Of(v.Kind)
	return resource.McpServer_builder{
		Name: &v.Name, Kind: &kind, Enabled: &v.Enabled,
		Command: &v.Command, Args: v.Args, Env: v.Env,
		Url: &v.Url, Headers: v.Headers,
	}.Build()
}

func runtimeMcpServer(v *resource.McpServer) *api.McpServer {
	if v == nil {
		return nil
	}
	return &api.McpServer{
		Name: v.GetName(), Kind: mcpkind.Name(v.GetKind()), Enabled: v.GetEnabled(),
		Command: v.GetCommand(), Args: v.GetArgs(), Env: v.GetEnv(),
		Url: v.GetUrl(), Headers: v.GetHeaders(),
	}
}
