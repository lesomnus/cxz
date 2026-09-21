package multiclient

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func (c *Client) Docker(ctx context.Context, in *api.DockerInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.DockerInput)
	reply, err := client.Docker(ctx, request, opts...)
	if err == nil && in.Action != "info" {
		c.refreshSource(ctx, "")
	}
	return reply, err
}

func (c *Client) FileMappings(ctx context.Context, in *api.FileMappingsInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.FileMappingsInput)
	reply, err := client.FileMappings(ctx, request, opts...)
	if err == nil {
		c.refreshSource(ctx, "")
	}
	return reply, err
}

func (c *Client) Create(ctx context.Context, in *api.CreateRequest, opts ...grpc.CallOption) (*api.Session, error) {
	name, id, client, err := c.route(ctx, in.Workspace)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.CreateRequest)
	request.Workspace = id
	reply, err := client.Create(ctx, request, opts...)
	if err == nil {
		c.refreshSource(ctx, in.Workspace)
	}
	return decorateSession(name, reply), err
}

func (c *Client) Get(ctx context.Context, in *api.SessionRef, opts ...grpc.CallOption) (*api.Session, error) {
	if err := c.waitReady(ctx, in.Id); err != nil {
		return nil, err
	}
	name, id, client, err := c.route(ctx, in.Id)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.SessionRef)
	request.Id = id
	reply, err := client.Get(ctx, request, opts...)
	return decorateSession(name, reply), err
}

func (c *Client) Memory(ctx context.Context, in *api.MemoryRequest, opts ...grpc.CallOption) (*api.MemoryReply, error) {
	_, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.MemoryRequest)
	request.SessionId = id
	reply, err := client.Memory(ctx, request, opts...)
	return reply, err
}

func (c *Client) Logs(ctx context.Context, in *api.LogsRequest, opts ...grpc.CallOption) (*api.LogsReply, error) {
	_, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.LogsRequest)
	request.SessionId = id
	reply, err := client.Logs(ctx, request, opts...)
	return reply, err
}

func (c *Client) Permission(ctx context.Context, in *api.PermissionInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	_, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.PermissionInput)
	request.SessionId = id
	reply, err := client.Permission(ctx, request, opts...)
	if err == nil {
		c.refreshSource(ctx, in.SessionId)
	}
	return reply, err
}

func (c *Client) Send(ctx context.Context, in *api.Input, opts ...grpc.CallOption) (*api.Receipt, error) {
	_, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.Input)
	request.SessionId = id
	reply, err := client.Send(ctx, request, opts...)
	if err == nil {
		c.refreshSource(ctx, in.SessionId)
	}
	return reply, err
}

func (c *Client) Attach(ctx context.Context, in *api.AttachmentInput, opts ...grpc.CallOption) (*api.Attachment, error) {
	_, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.AttachmentInput)
	request.SessionId = id
	reply, err := client.Attach(ctx, request, opts...)

	return reply, err
}

func (c *Client) Activity(ctx context.Context, in *api.ActivityInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	_, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.ActivityInput)
	request.SessionId = id
	reply, err := client.Activity(ctx, request, opts...)

	return reply, err
}

func (c *Client) UpdateAgent(ctx context.Context, in *api.AgentUpdateInput, opts ...grpc.CallOption) (*api.AgentUpdateStatus, error) {
	_, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.AgentUpdateInput)
	request.SessionId = id
	reply, err := client.UpdateAgent(ctx, request, opts...)
	if err == nil {
		c.refreshSource(ctx, in.SessionId)
	}
	return reply, err
}

func (c *Client) Reply(ctx context.Context, in *api.Answer, opts ...grpc.CallOption) (*api.Receipt, error) {
	_, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.Answer)
	request.SessionId = id
	reply, err := client.Reply(ctx, request, opts...)
	if err == nil {
		c.refreshSource(ctx, in.SessionId)
	}
	return reply, err
}

func (c *Client) Interrupt(ctx context.Context, in *api.Control, opts ...grpc.CallOption) (*api.Receipt, error) {
	_, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.Control)
	request.SessionId = id
	reply, err := client.Interrupt(ctx, request, opts...)
	if err == nil {
		c.refreshSource(ctx, in.SessionId)
	}
	return reply, err
}

func (c *Client) Resume(ctx context.Context, in *api.Control, opts ...grpc.CallOption) (*api.Session, error) {
	name, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.Control)
	request.SessionId = id
	reply, err := client.Resume(ctx, request, opts...)
	if err == nil {
		c.refreshSource(ctx, in.SessionId)
	}
	return decorateSession(name, reply), err
}

func (c *Client) Stop(ctx context.Context, in *api.Control, opts ...grpc.CallOption) (*api.Receipt, error) {
	_, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.Control)
	request.SessionId = id
	reply, err := client.Stop(ctx, request, opts...)
	if err == nil {
		c.refreshSource(ctx, in.SessionId)
	}
	return reply, err
}

func (c *Client) History(ctx context.Context, in *api.WatchRequest, opts ...grpc.CallOption) (*api.EventBatch, error) {
	name, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.WatchRequest)
	request.SessionId = id
	reply, err := client.History(ctx, request, opts...)
	if reply != nil {
		reply = proto.Clone(reply).(*api.EventBatch)
		for _, e := range reply.Events {
			e.SessionId = Scope(name, e.SessionId)
		}
	}
	return reply, err
}

func (c *Client) Background(ctx context.Context, in *api.SessionRef, opts ...grpc.CallOption) (*api.BackgroundReply, error) {
	_, id, client, err := c.route(ctx, in.Id)
	if err != nil {
		return nil, err
	}
	return client.Background(ctx, &api.SessionRef{Id: id}, opts...)
}

func (c *Client) Open(ctx context.Context, in *api.ProjectRequest, opts ...grpc.CallOption) (*api.Session, error) {
	name, id, client, err := c.route(ctx, in.Workspace)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.ProjectRequest)
	request.Workspace = id
	reply, err := client.Open(ctx, request, opts...)
	if err == nil {
		c.refreshSource(ctx, in.Workspace)
	}
	return decorateSession(name, reply), err
}

func (c *Client) Down(ctx context.Context, in *api.ProjectRequest, opts ...grpc.CallOption) (*api.Receipt, error) {
	_, id, client, err := c.route(ctx, in.Workspace)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.ProjectRequest)
	request.Workspace = id
	reply, err := client.Down(ctx, request, opts...)
	if err == nil {
		c.refreshSource(ctx, in.Workspace)
	}
	return reply, err
}

func (c *Client) CopyMemory(ctx context.Context, in *api.CopyMemoryRequest, opts ...grpc.CallOption) (*api.Receipt, error) {
	name, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	target, raw, _, err := c.route(ctx, in.TargetId)
	if err != nil {
		return nil, err
	}
	if name != target {
		return nil, fmt.Errorf("memory copy between connections is not supported; select a session via %s", name)
	}
	request := proto.Clone(in).(*api.CopyMemoryRequest)
	request.SessionId = id
	request.TargetId = raw
	return client.CopyMemory(ctx, request, opts...)
}
func (c *Client) Watch(ctx context.Context, in *api.WatchRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[api.Event], error) {
	name, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.WatchRequest)
	request.SessionId = id
	stream, err := client.Watch(ctx, request, opts...)
	if err != nil {
		return nil, err
	}
	return eventStream{ServerStreamingClient: stream, name: name}, nil
}
func (c *Client) RenameSession(ctx context.Context, ref, alias string) (*api.Session, error) {
	name, id, client, err := c.route(ctx, ref)
	if err != nil {
		return nil, err
	}
	r, ok := client.(interface {
		RenameSession(context.Context, string, string) (*api.Session, error)
	})
	if !ok {
		return nil, fmt.Errorf("session rename unavailable")
	}
	s, err := r.RenameSession(ctx, id, alias)
	if err == nil {
		c.refreshSource(ctx, ref)
	}
	return decorateSession(name, s), err
}
func (c *Client) DeleteSession(ctx context.Context, ref string) error {
	_, id, client, err := c.route(ctx, ref)
	if err != nil {
		return err
	}
	r, ok := client.(interface {
		DeleteSession(context.Context, string) error
	})
	if !ok {
		return fmt.Errorf("session deletion unavailable")
	}
	err = r.DeleteSession(ctx, id)
	if err == nil {
		c.refreshSource(ctx, ref)
	}
	return err
}
func (c *Client) refreshSource(ctx context.Context, ref string) {
	name, _ := c.routeName(ctx, ref)
	if s := c.sources[name]; s != nil {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}
