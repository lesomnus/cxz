package resourceclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

// The resource service resolves the account and its binding itself; a client
// names only the account. Backend and binding in the runtime input are ignored.
func (c *Client) SwitchSessionAccount(ctx context.Context, r *api.SessionAccountInput, opts ...grpc.CallOption) (*api.Session, error) {
	v, err := c.sessions.SwitchAccount(ctx, resource.SessionSwitchAccountRequest_builder{Ref: sr(r.SessionId), Account: resource.AccountRef_builder{Alias: &r.Account}.Build()}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return c.view(ctx, v)
}
