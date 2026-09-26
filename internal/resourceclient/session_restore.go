package resourceclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
)

func (c *Client) ArchivedSessions(ctx context.Context, project string) ([]*api.Session, error) {
	var out []*api.Session
	after := ""
	for {
		page, err := c.sessions.List(ctx, resource.SessionListRequest_builder{Filters: []*resource.SessionFilter{resource.SessionFilter_builder{Project: pr(project), Listed: ptr(false)}.Build()}, Size: 200, After: after}.Build())
		if err != nil {
			return nil, err
		}
		for _, item := range page.GetItems() {
			v, err := c.view(ctx, item)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		after = page.GetNext()
		if after == "" {
			return out, nil
		}
	}
}

func (c *Client) RestoreSession(ctx context.Context, id string) (*api.Session, error) {
	v, err := c.sessions.Restore(ctx, sr(id))
	if err != nil {
		return nil, err
	}
	return c.view(ctx, v)
}
