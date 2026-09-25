package resourceclient

import (
	"bytes"
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/resource"
)

func (c *Client) UploadAttachment(ctx context.Context, in assets.Upload, src io.Reader) (string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := c.sessions.Upload(ctx)
	if err != nil {
		return "", err
	}
	finish := func() (string, error) {
		result, err := stream.CloseAndRecv()
		if err != nil {
			return "", err
		}
		return result.GetPath(), nil
	}
	if err := stream.Send(resource.SessionUploadRequest_builder{Ref: sr(in.SessionID), RunId: &in.RunID, Name: &in.Name, Size: &in.Size}.Build()); err != nil {
		if err == io.EOF {
			return finish()
		}
		return "", err
	}
	buffer := make([]byte, 256*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := src.Read(buffer)
		if n > 0 {
			if err := stream.Send(resource.SessionUploadRequest_builder{Content: bytes.Clone(buffer[:n])}.Build()); err != nil {
				if err == io.EOF {
					return finish()
				}
				return "", err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return finish()
}
