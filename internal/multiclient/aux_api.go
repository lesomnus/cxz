package multiclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// Routing an aux call reads one field. The old envelope made this layer parse
// the request body to find the session a task belonged to, and then serialise
// it again with the connection prefix removed; a typed field is the same
// information without the detour.

func (c *Client) AuxRun(ctx context.Context, r *api.AuxRunInput, opts ...grpc.CallOption) (*api.AuxState, error) {
	in := proto.Clone(r).(*api.AuxRunInput)
	_, id, client, err := c.route(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	in.SessionId = id
	return client.AuxRun(ctx, in, opts...)
}

func (c *Client) AuxStatus(ctx context.Context, r *api.AuxStatusInput, opts ...grpc.CallOption) (*api.AuxState, error) {
	in := proto.Clone(r).(*api.AuxStatusInput)
	_, id, client, err := c.route(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	in.SessionId = id
	return client.AuxStatus(ctx, in, opts...)
}

func (c *Client) AuxPrefer(ctx context.Context, r *api.AuxPreferInput, opts ...grpc.CallOption) (*api.AuxState, error) {
	in := proto.Clone(r).(*api.AuxPreferInput)
	_, id, client, err := c.route(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	in.SessionId = id
	return client.AuxPrefer(ctx, in, opts...)
}

func (c *Client) AuxCancel(ctx context.Context, r *api.AuxCancelInput, opts ...grpc.CallOption) (*api.AuxState, error) {
	in := proto.Clone(r).(*api.AuxCancelInput)
	_, id, client, err := c.route(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	in.SessionId = id
	return client.AuxCancel(ctx, in, opts...)
}

func (c *Client) AuxForget(ctx context.Context, r *api.AuxForgetInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	in := proto.Clone(r).(*api.AuxForgetInput)
	_, id, client, err := c.route(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	in.SessionId = id
	return client.AuxForget(ctx, in, opts...)
}

// The configuration is the installation's, so these go to the connection the
// caller is looking at rather than to a session's own.

func (c *Client) AuxConfig(ctx context.Context, r *api.Empty, opts ...grpc.CallOption) (*api.AuxConfigReply, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	return client.AuxConfig(ctx, r, opts...)
}

func (c *Client) AuxSetConfig(ctx context.Context, r *api.AuxSetConfigInput, opts ...grpc.CallOption) (*api.AuxConfigReply, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	return client.AuxSetConfig(ctx, r, opts...)
}

func (c *Client) AuxModels(ctx context.Context, r *api.AuxModelsInput, opts ...grpc.CallOption) (*api.AuxModelsReply, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	return client.AuxModels(ctx, r, opts...)
}

func (c *Client) AuxLoginInfo(ctx context.Context, r *api.AuxLoginInfoInput, opts ...grpc.CallOption) (*api.AuxLoginInfoReply, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	return client.AuxLoginInfo(ctx, r, opts...)
}
