package multiclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func (c *Client) Devcontainer(ctx context.Context, in *api.DevcontainerInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	return client.Devcontainer(ctx, proto.Clone(in).(*api.DevcontainerInput), opts...)
}
