package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/mcpkind"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A registration belongs to the installation, so it lives on the project
// service, scoped by an optional ref the same way the skills library is. The
// two calls about one session's connection are on the session service, because
// that is whose connection it is.

func (s ProjectServer) GetMcpServers(ctx context.Context, r *resource.McpServersRequest) (*resource.McpServersReply, error) {
	project, err := s.projectScope(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return mcpServersReply(s.shared.runtime.GetMcpServers(ctx, &api.McpServersInput{Project: project}))
}

func (s ProjectServer) PutMcpServer(ctx context.Context, r *resource.PutMcpServerRequest) (*resource.McpServersReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	server, err := runtimeMcpServer(r.GetServer())
	if err != nil {
		return nil, err
	}
	return mcpServersReply(s.shared.runtime.PutMcpServer(ctx, &api.PutMcpServerInput{
		Id: r.GetId(), Server: server,
	}))
}

func (s ProjectServer) RemoveMcpServer(ctx context.Context, r *resource.McpServerRequest) (*resource.McpServersReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return mcpServersReply(s.shared.runtime.RemoveMcpServer(ctx, &api.McpServerInput{Id: r.GetId()}))
}

func (s ProjectServer) SetMcpServerDefault(ctx context.Context, r *resource.McpServerDefaultRequest) (*resource.McpServersReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return mcpServersReply(s.shared.runtime.SetMcpServerDefault(ctx, &api.McpServerDefaultInput{
		Id: r.GetId(), Enabled: r.GetEnabled(),
	}))
}

func (s ProjectServer) SetProjectMcpServer(ctx context.Context, r *resource.ProjectMcpServerRequest) (*resource.McpServersReply, error) {
	project, err := s.changeScope(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return mcpServersReply(s.shared.runtime.SetProjectMcpServer(ctx, &api.ProjectMcpServerInput{
		Project: project, Id: r.GetId(), Enabled: r.GetEnabled(),
	}))
}

func (s ProjectServer) ClearProjectMcpServer(ctx context.Context, r *resource.ClearProjectMcpServerRequest) (*resource.McpServersReply, error) {
	project, err := s.changeScope(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return mcpServersReply(s.shared.runtime.ClearProjectMcpServer(ctx, &api.ClearProjectMcpServerInput{
		Project: project, Id: r.GetId(),
	}))
}

func (s ProjectServer) McpSessions(ctx context.Context, r *resource.McpSessionsRequest) (*resource.McpSessionsReply, error) {
	project, err := s.projectScope(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	if project == "" {
		return nil, status.Error(codes.InvalidArgument, "a session status needs a project")
	}
	out, err := s.shared.runtime.McpSessions(ctx, &api.McpSessionsInput{Project: project})
	if err != nil {
		return nil, err
	}
	reply := resource.McpSessionsReply_builder{Message: &out.Message}
	for _, v := range out.Sessions {
		reply.Sessions = append(reply.Sessions, resource.McpSessionStatus_builder{
			Session: &v.SessionId, Title: &v.Title, Pending: &v.Pending,
			LaunchDigest: &v.LaunchDigest, Servers: v.Servers,
		}.Build())
	}
	return reply.Build(), nil
}

func (s ProjectServer) SyncMcpServers(ctx context.Context, r *resource.SyncMcpServersRequest) (*resource.SyncMcpServersReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	servers := map[string]*api.McpServer{}
	for id, v := range r.GetServers() {
		server, err := runtimeMcpServer(v)
		if err != nil {
			return nil, err
		}
		servers[id] = server
	}
	out, err := s.shared.runtime.SyncMcpServers(ctx, &api.SyncMcpServersInput{Servers: servers})
	if err != nil {
		return nil, err
	}
	return resource.SyncMcpServersReply_builder{Status: &out.Status}.Build(), nil
}

func (s SessionServer) McpLogs(ctx context.Context, r *resource.SessionMcpRequest) (*resource.SessionMcpLogsReply, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.McpLogs(ctx, &api.McpLogsInput{SessionId: v.GetRuntimeId(), Id: r.GetId()})
	if err != nil {
		return nil, err
	}
	return resource.SessionMcpLogsReply_builder{Text: &out.Text, Message: &out.Message}.Build(), nil
}

func (s SessionServer) RestartMcp(ctx context.Context, r *resource.SessionMcpRequest) (*resource.SessionReceipt, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.RestartMcp(ctx, &api.RestartMcpInput{SessionId: v.GetRuntimeId(), Id: r.GetId()})
	if err != nil {
		return nil, err
	}
	return resource.SessionReceipt_builder{Status: &out.Status}.Build(), nil
}

// runtimeMcpServer refuses a kind this build does not know rather than storing
// a definition as something it is not. The enum is what makes that a check on
// one field instead of a string comparison in the store.
func runtimeMcpServer(v *resource.McpServer) (*api.McpServer, error) {
	if v == nil {
		return nil, status.Error(codes.InvalidArgument, "an MCP registration needs a definition")
	}
	kind := mcpkind.Name(v.GetKind())
	if kind == "" {
		return nil, status.Error(codes.InvalidArgument, "unknown MCP kind")
	}
	return &api.McpServer{
		Name: v.GetName(), Kind: kind, Enabled: v.GetEnabled(),
		Command: v.GetCommand(), Args: v.GetArgs(), Env: v.GetEnv(),
		Url: v.GetUrl(), Headers: v.GetHeaders(),
	}, nil
}

func mcpServersReply(v *api.McpServersReply, err error) (*resource.McpServersReply, error) {
	if err != nil {
		return nil, err
	}
	out := resource.McpServersReply_builder{Message: &v.Message}
	for _, e := range v.Entries {
		out.Entries = append(out.Entries, resource.McpEntry_builder{
			Id: &e.Id, Server: resourceMcpServer(e.Server),
			Override: e.Override, Effective: &e.Effective,
		}.Build())
	}
	return out.Build(), nil
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
