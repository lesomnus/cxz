package multiclient

import (
	"context"
	"fmt"
	"io"

	"github.com/lesomnus/cxz/internal/assets"
)

func (c *Client) UploadAttachment(ctx context.Context, in assets.Upload, src io.Reader) (string, error) {
	_, id, client, err := c.route(ctx, in.SessionID)
	if err != nil {
		return "", err
	}
	uploader, ok := client.(assets.Uploader)
	if !ok {
		return "", fmt.Errorf("connection does not support file uploads; update cxz")
	}
	in.SessionID = id
	return uploader.UploadAttachment(ctx, in, src)
}
