package resourceclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

func (c *Client) Devcontainer(ctx context.Context, r *api.DevcontainerInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	out, err := c.projects.Devcontainer(ctx, resource.DevcontainerRequest_builder{Spec: r.Spec}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.Receipt{Status: out.GetStatus()}, nil
}
