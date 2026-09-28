package multiclient

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/containerterm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) Download(ctx context.Context, project, path string, dst io.Writer) error {
	_, id, client, err := c.route(ctx, project)
	if err != nil {
		return err
	}
	if client, ok := client.(containerterm.DownloadClient); ok {
		return client.Download(ctx, id, path, dst)
	}
	return status.Error(codes.Unimplemented, "container downloads unavailable; update the manager")
}
