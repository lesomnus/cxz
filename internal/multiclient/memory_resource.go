package multiclient

import (
	"context"
	"github.com/lesomnus/cxz/internal/memoryclient"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) MemoryResources(ctx context.Context, project string) (resource.MemoryServiceClient, string, error) {
	_, id, client, err := c.route(ctx, project)
	if err != nil {
		return nil, "", err
	}
	if m, ok := client.(memoryclient.Client); ok {
		return m.MemoryResources(ctx, id)
	}
	return nil, "", status.Error(codes.Unimplemented, "connection does not provide MemoryService")
}
