package multiclient

import (
	"context"

	"github.com/lesomnus/cxz/internal/containerterm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) Paths(ctx context.Context, project, path string, emit func(containerterm.PathListing)) (containerterm.PathListing, error) {
	_, id, client, err := c.route(ctx, project)
	if err != nil {
		return containerterm.PathListing{}, err
	}
	if client, ok := client.(containerterm.PathClient); ok {
		return client.Paths(ctx, id, path, emit)
	}
	return containerterm.PathListing{}, status.Error(codes.Unimplemented, "container path browsing unavailable")
}
