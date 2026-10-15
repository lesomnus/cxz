package resourceclient

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

func (c *Client) ForkSession(ctx context.Context, r *api.SessionForkInput, opts ...grpc.CallOption) (*api.Session, error) {
	child, err := c.sessions.Fork(ctx, resource.SessionControl_builder{Ref: sr(r.SessionId), RunId: &r.RunId, ClientId: &r.ClientId}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return c.view(ctx, child)
}
