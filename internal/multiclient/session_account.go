package multiclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func (c *Client) SwitchSessionAccount(ctx context.Context, r *api.SessionAccountInput, opts ...grpc.CallOption) (*api.Session, error) {
	name, id, client, err := c.route(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(r).(*api.SessionAccountInput)
	request.SessionId = id
	v, err := client.SwitchSessionAccount(ctx, request, opts...)
	return decorateSession(name, v), err
}
