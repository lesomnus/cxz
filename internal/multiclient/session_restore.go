package multiclient

import (
	"context"
	"fmt"

	"github.com/lesomnus/cxz/api"
)

func (c *Client) ArchivedSessions(ctx context.Context, project string) ([]*api.Session, error) {
	name, id, client, err := c.route(ctx, project)
	if err != nil {
		return nil, err
	}
	r, ok := client.(interface {
		ArchivedSessions(context.Context, string) ([]*api.Session, error)
	})
	if !ok {
		return nil, fmt.Errorf("session restoration unavailable; update the host manager")
	}
	items, err := r.ArchivedSessions(ctx, id)
	for i, item := range items {
		items[i] = decorateSession(name, item)
	}
	return items, err
}

func (c *Client) RestoreSession(ctx context.Context, ref string) (*api.Session, error) {
	name, id, client, err := c.route(ctx, ref)
	if err != nil {
		return nil, err
	}
	r, ok := client.(interface {
		RestoreSession(context.Context, string) (*api.Session, error)
	})
	if !ok {
		return nil, fmt.Errorf("session restoration unavailable; update the host manager")
	}
	v, err := r.RestoreSession(ctx, id)
	if err == nil {
		c.refreshSource(ctx, ref)
	}
	return decorateSession(name, v), err
}
