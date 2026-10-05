package resourceclient

import (
	"context"
	"github.com/lesomnus/cxz/resource"
)

func (c *Client) RenameProject(ctx context.Context, id, title string) error {
	_, err := c.projects.Patch(ctx, resource.ProjectPatchRequest_builder{Ref: pr(id), Name: &title}.Build())
	return err
}
