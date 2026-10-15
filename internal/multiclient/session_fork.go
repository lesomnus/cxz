package multiclient

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func (c *Client) ForkSession(ctx context.Context, r *api.SessionForkInput, opts ...grpc.CallOption) (*api.Session, error) {
	name, id, client, err := c.route(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(r).(*api.SessionForkInput)
	request.SessionId = id
	child, err := client.ForkSession(ctx, request, opts...)
	if err == nil {
		c.refreshSource(ctx, r.SessionId)
	}
	return decorateSession(name, child), err
}
