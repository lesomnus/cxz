package resourceclient

import "context"

func (c *Client) DeleteSession(ctx context.Context, id string) error {
	_, err := c.sessions.Erase(ctx, sr(id))
	return err
}
func (c *Client) DeleteProject(ctx context.Context, handle string) error {
	p, err := c.ResolveProject(ctx, handle)
	if err != nil {
		return err
	}
	_, err = c.projects.Erase(ctx, pr(p.Id))
	return err
}
