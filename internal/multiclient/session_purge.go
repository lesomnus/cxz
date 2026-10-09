package multiclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// A purge names a session, and the session names the connection that holds it.
func (c *Client) PurgeSession(ctx context.Context, r *api.SessionPurgeInput, opts ...grpc.CallOption) (*api.SessionPurgeReply, error) {
	in := proto.Clone(r).(*api.SessionPurgeInput)
	_, id, client, err := c.route(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	in.SessionId = id
	out, err := client.PurgeSession(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	// The caller addressed a session by its prefixed name and reads the reply
	// against that, so what comes back keeps the name that was asked about.
	out.SessionId = r.SessionId
	return out, nil
}

// The budgets are an installation's, so they follow the connection being looked
// at rather than a session of their own.
func (c *Client) GetHistoryPolicy(ctx context.Context, r *api.Empty, opts ...grpc.CallOption) (*api.HistoryPolicy, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	return client.GetHistoryPolicy(ctx, r, opts...)
}

func (c *Client) SetHistoryPolicy(ctx context.Context, r *api.HistoryPolicy, opts ...grpc.CallOption) (*api.HistoryPolicy, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	return client.SetHistoryPolicy(ctx, r, opts...)
}

func (c *Client) MarkHistoryTrimmable(ctx context.Context, r *api.Empty, opts ...grpc.CallOption) (*api.Empty, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	return client.MarkHistoryTrimmable(ctx, r, opts...)
}

// A trim floor is a session's, and a session names the connection that holds
// it. It is routed like every other session read rather than refused here:
// resolving it is what the resource API does now.
func (c *Client) GetHistoryFloor(ctx context.Context, r *api.SessionRef, opts ...grpc.CallOption) (*api.HistoryFloorReply, error) {
	_, id, client, err := c.route(ctx, r.Id)
	if err != nil {
		return nil, err
	}
	return client.GetHistoryFloor(ctx, &api.SessionRef{Id: id}, opts...)
}
