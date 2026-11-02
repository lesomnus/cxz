package resourceclient

import (
	"context"
	"github.com/lesomnus/cxz/resource"
)

func (c *Client) MemoryResources(_ context.Context, project string) (resource.MemoryServiceClient, string, error) {
	return resource.NewMemoryServiceClient(c.connection), project, nil
}
