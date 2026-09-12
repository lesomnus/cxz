package resourceclient

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
)

func (c *Client) RenameSession(ctx context.Context, id, alias string) (*api.Session, error) {
	s, err := c.sessions.Patch(ctx, resource.SessionPatchRequest_builder{Ref: sr(id), Alias: &alias}.Build())
	if err != nil {
		return nil, err
	}
	return c.view(ctx, s)
}
