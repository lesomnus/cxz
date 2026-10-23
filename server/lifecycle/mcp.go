package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/mcpkind"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A registration belongs to the installation, not to a project, so it has its
// own service -- `in` chooses whose view comes back and never what to write,
// the same way the skills library does. What is *running* from a registration
// is a different resource again: McpConnectionService observes it, and the two
// calls about one session's own connection stay on the session service, because
// that is whose connection it is. See docs/plans/service-boundaries.md.
type McpServerServer struct {
	Layer
	resource.UnimplementedMcpServerServiceServer
}

type McpConnectionServer struct {
	Layer
	resource.UnimplementedMcpConnectionServiceServer
}

func (s Layer) McpServer() resource.McpServerServiceServer { return McpServerServer{Layer: s} }

func (s Layer) McpConnection() resource.McpConnectionServiceServer {
	return McpConnectionServer{Layer: s}
}

func (s McpServerServer) List(ctx context.Context, r *resource.McpServerListRequest) (*resource.McpServerList, error) {
	return mcpServersReply(s.shared.runtime.GetMcpServers(ctx, &api.McpServersInput{Project: r.GetIn()}))
}

func (s McpServerServer) Put(ctx context.Context, r *resource.McpServerPutRequest) (*resource.McpServerList, error) {
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

func (s McpServerServer) Remove(ctx context.Context, r *resource.McpServerRemoveRequest) (*resource.McpServerList, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return mcpServersReply(s.shared.runtime.RemoveMcpServer(ctx, &api.McpServerInput{Id: r.GetId()}))
}

func (s McpServerServer) SetDefault(ctx context.Context, r *resource.McpServerDefaultRequest) (*resource.McpServerList, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return mcpServersReply(s.shared.runtime.SetMcpServerDefault(ctx, &api.McpServerDefaultInput{
		Id: r.GetId(), Enabled: r.GetEnabled(),
	}))
}

func (s McpServerServer) Enable(ctx context.Context, r *resource.McpServerEnableRequest) (*resource.McpServerList, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return mcpServersReply(s.shared.runtime.SetProjectMcpServer(ctx, &api.ProjectMcpServerInput{
		Project: r.GetIn(), Id: r.GetId(), Enabled: r.GetEnabled(),
	}))
}

func (s McpServerServer) Clear(ctx context.Context, r *resource.McpServerClearRequest) (*resource.McpServerList, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return mcpServersReply(s.shared.runtime.ClearProjectMcpServer(ctx, &api.ClearProjectMcpServerInput{
		Project: r.GetIn(), Id: r.GetId(),
	}))
}

func (s McpConnectionServer) List(ctx context.Context, r *resource.McpConnectionListRequest) (*resource.McpConnectionList, error) {
	if r.GetProject() == "" {
		return nil, status.Error(codes.InvalidArgument, "a connection list needs a project")
	}
	out, err := s.shared.runtime.McpSessions(ctx, &api.McpSessionsInput{Project: r.GetProject()})
	if err != nil {
		return nil, err
	}
	reply := resource.McpConnectionList_builder{Message: &out.Message}
	for _, v := range out.Sessions {
		reply.Connections = append(reply.Connections, resource.McpConnection_builder{
			Session: &v.SessionId, Title: &v.Title, Pending: &v.Pending,
			LaunchDigest: &v.LaunchDigest, Servers: v.Servers,
		}.Build())
	}
	return reply.Build(), nil
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

func mcpServersReply(v *api.McpServersReply, err error) (*resource.McpServerList, error) {
	if err != nil {
		return nil, err
	}
	out := resource.McpServerList_builder{Message: &v.Message}
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
