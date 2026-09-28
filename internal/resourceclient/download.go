package resourceclient

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/resource"
)

func (c *Client) Download(ctx context.Context, project, path string, dst io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := c.projects.Download(ctx, resource.ProjectDownloadRequest_builder{Ref: pr(project), Path: &path}.Build())
	if err != nil {
		return err
	}
	for {
		reply, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if reply.HasTotalSize() {
			if err := containerterm.ReportDownloadSize(dst, reply.GetTotalSize()); err != nil {
				return err
			}
		}
		b := reply.GetData()
		if len(b) == 0 {
			continue
		}
		n, err := dst.Write(b)
		if err != nil {
			return err
		}
		if n != len(b) {
			return io.ErrShortWrite
		}
	}
}
