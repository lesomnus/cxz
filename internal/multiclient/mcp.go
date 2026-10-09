package multiclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Registrations belong to one installation, so an MCP call follows the
// connection its project names -- or the one being looked at, when it names
// none. The project and the session travel in fields now, so routing reads them
// instead of parsing a payload to find them and writing it back out.

func (c *Client) GetMcpServers(ctx context.Context, r *api.McpServersInput, opts ...grpc.CallOption) (*api.McpServersReply, error) {
	in := proto.Clone(r).(*api.McpServersInput)
	_, id, client, err := c.route(ctx, r.Project)
	if err != nil {
		return nil, err
	}
	in.Project = id
	return client.GetMcpServers(ctx, in, opts...)
}

func (c *Client) PutMcpServer(ctx context.Context, r *api.PutMcpServerInput, opts ...grpc.CallOption) (*api.McpServersReply, error) {
	return c.changeMcp(ctx, "", func(client api.SessionsClient, _ string) (*api.McpServersReply, error) {
		return client.PutMcpServer(ctx, proto.Clone(r).(*api.PutMcpServerInput), opts...)
	})
}

func (c *Client) RemoveMcpServer(ctx context.Context, r *api.McpServerInput, opts ...grpc.CallOption) (*api.McpServersReply, error) {
	return c.changeMcp(ctx, "", func(client api.SessionsClient, _ string) (*api.McpServersReply, error) {
		return client.RemoveMcpServer(ctx, proto.Clone(r).(*api.McpServerInput), opts...)
	})
}

func (c *Client) SetMcpServerDefault(ctx context.Context, r *api.McpServerDefaultInput, opts ...grpc.CallOption) (*api.McpServersReply, error) {
	return c.changeMcp(ctx, "", func(client api.SessionsClient, _ string) (*api.McpServersReply, error) {
		return client.SetMcpServerDefault(ctx, proto.Clone(r).(*api.McpServerDefaultInput), opts...)
	})
}

func (c *Client) SetProjectMcpServer(ctx context.Context, r *api.ProjectMcpServerInput, opts ...grpc.CallOption) (*api.McpServersReply, error) {
	return c.changeMcp(ctx, r.Project, func(client api.SessionsClient, id string) (*api.McpServersReply, error) {
		in := proto.Clone(r).(*api.ProjectMcpServerInput)
		in.Project = id
		return client.SetProjectMcpServer(ctx, in, opts...)
	})
}

func (c *Client) ClearProjectMcpServer(ctx context.Context, r *api.ClearProjectMcpServerInput, opts ...grpc.CallOption) (*api.McpServersReply, error) {
	return c.changeMcp(ctx, r.Project, func(client api.SessionsClient, id string) (*api.McpServersReply, error) {
		in := proto.Clone(r).(*api.ClearProjectMcpServerInput)
		in.Project = id
		return client.ClearProjectMcpServer(ctx, in, opts...)
	})
}

func (c *Client) McpSessions(ctx context.Context, r *api.McpSessionsInput, opts ...grpc.CallOption) (*api.McpSessionsReply, error) {
	in := proto.Clone(r).(*api.McpSessionsInput)
	name, id, client, err := c.route(ctx, r.Project)
	if err != nil {
		return nil, err
	}
	in.Project = id
	out, err := client.McpSessions(ctx, in, opts...)
	if out != nil {
		out = proto.Clone(out).(*api.McpSessionsReply)
		// The caller addresses sessions by their prefixed names everywhere
		// else, so these are prefixed too rather than being the only session
		// ids it cannot hand back.
		for _, v := range out.Sessions {
			v.SessionId = Scope(name, v.SessionId)
		}
	}
	return out, err
}

func (c *Client) McpLogs(ctx context.Context, r *api.McpLogsInput, opts ...grpc.CallOption) (*api.McpLogsReply, error) {
	in := proto.Clone(r).(*api.McpLogsInput)
	_, id, client, err := c.route(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	in.SessionId = id
	return client.McpLogs(ctx, in, opts...)
}

func (c *Client) RestartMcp(ctx context.Context, r *api.RestartMcpInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	in := proto.Clone(r).(*api.RestartMcpInput)
	_, id, client, err := c.route(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	in.SessionId = id
	return client.RestartMcp(ctx, in, opts...)
}

// SyncMcpServers runs from a manager to a project container. A frontend
// connection is on the far side of that, so it says so rather than reaching for
// a manager that would deliver to itself.
func (c *Client) SyncMcpServers(context.Context, *api.SyncMcpServersInput, ...grpc.CallOption) (*api.Receipt, error) {
	return nil, status.Error(codes.Unimplemented, "MCP settings are delivered by a manager to a project, not through a frontend connection")
}

// A change decides what a project may launch with, so the connection that made
// it is refreshed; a read is not.
func (c *Client) changeMcp(ctx context.Context, ref string, call func(api.SessionsClient, string) (*api.McpServersReply, error)) (*api.McpServersReply, error) {
	_, id, client, err := c.route(ctx, ref)
	if err != nil {
		return nil, err
	}
	out, err := call(client, id)
	if err == nil {
		c.refreshSource(ctx, ref)
	}
	return out, err
}
