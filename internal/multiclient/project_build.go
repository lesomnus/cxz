package multiclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func (c *Client) BuildProject(ctx context.Context, in *api.BuildProjectInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	_, id, client, err := c.route(ctx, in.Workspace)
	if err != nil {
		return nil, err
	}
	request := proto.Clone(in).(*api.BuildProjectInput)
	request.Workspace = id
	reply, err := client.BuildProject(ctx, request, opts...)
	if err == nil {
		c.refreshSource(ctx, in.Workspace)
	}
	return reply, err
}
