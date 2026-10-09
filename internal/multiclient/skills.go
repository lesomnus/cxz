package multiclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// A library belongs to one installation, so a skills call follows the
// connection its project names -- or the one being looked at, when it names
// none. The project travels in a field now, so routing reads it instead of
// parsing a payload to find it.

func (c *Client) GetSkills(ctx context.Context, r *api.SkillsInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	in := proto.Clone(r).(*api.SkillsInput)
	_, id, client, err := c.route(ctx, r.Project)
	if err != nil {
		return nil, err
	}
	in.Project = id
	return client.GetSkills(ctx, in, opts...)
}

func (c *Client) AddSkill(ctx context.Context, r *api.SkillInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	return c.changeSkill(ctx, r.Project, func(client api.SessionsClient, id string) (*api.SkillsReply, error) {
		in := proto.Clone(r).(*api.SkillInput)
		in.Project = id
		return client.AddSkill(ctx, in, opts...)
	})
}

func (c *Client) RemoveSkill(ctx context.Context, r *api.SkillInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	return c.changeSkill(ctx, r.Project, func(client api.SessionsClient, id string) (*api.SkillsReply, error) {
		in := proto.Clone(r).(*api.SkillInput)
		in.Project = id
		return client.RemoveSkill(ctx, in, opts...)
	})
}

func (c *Client) SetSkillDefault(ctx context.Context, r *api.SkillDefaultInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	return c.changeSkill(ctx, "", func(client api.SessionsClient, _ string) (*api.SkillsReply, error) {
		return client.SetSkillDefault(ctx, proto.Clone(r).(*api.SkillDefaultInput), opts...)
	})
}

func (c *Client) SetProjectSkill(ctx context.Context, r *api.ProjectSkillInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	return c.changeSkill(ctx, r.Project, func(client api.SessionsClient, id string) (*api.SkillsReply, error) {
		in := proto.Clone(r).(*api.ProjectSkillInput)
		in.Project = id
		return client.SetProjectSkill(ctx, in, opts...)
	})
}

func (c *Client) ClearProjectSkill(ctx context.Context, r *api.ClearProjectSkillInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	return c.changeSkill(ctx, r.Project, func(client api.SessionsClient, id string) (*api.SkillsReply, error) {
		in := proto.Clone(r).(*api.ClearProjectSkillInput)
		in.Project = id
		return client.ClearProjectSkill(ctx, in, opts...)
	})
}

// A change decides what a running project may see, so the connection that made
// it is refreshed; a read is not.
func (c *Client) changeSkill(ctx context.Context, ref string, call func(api.SessionsClient, string) (*api.SkillsReply, error)) (*api.SkillsReply, error) {
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

// SyncSkills is the manager's call to a project container. A frontend
// connection is on the other side of that, so there is nothing here to deliver
// to.
func (c *Client) SyncSkills(context.Context, *api.SyncSkillsInput, ...grpc.CallOption) (*api.Receipt, error) {
	return nil, status.Error(codes.Unimplemented, "skills are delivered by a manager to a project, not through a frontend connection")
}

func (c *Client) RenderDevcontainer(ctx context.Context, r *api.RenderDevcontainerInput, opts ...grpc.CallOption) (*api.RenderDevcontainerReply, error) {
	in := proto.Clone(r).(*api.RenderDevcontainerInput)
	_, id, client, err := c.route(ctx, r.Handle)
	if err != nil {
		return nil, err
	}
	in.Handle = id
	return client.RenderDevcontainer(ctx, in, opts...)
}
