package resourceclient

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/resource"
)

func (c *Client) Paths(ctx context.Context, project, path string, emit func(containerterm.PathListing)) (containerterm.PathListing, error) {
	var out containerterm.PathListing
	stream, err := c.projects.Paths(ctx, resource.ProjectPathsRequest_builder{Ref: pr(project), Path: &path}.Build())
	if err != nil {
		return out, err
	}
	for {
		r, err := stream.Recv()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		for _, e := range r.GetEntries() {
			out.Entries = append(out.Entries, containerterm.PathEntry{Name: e.GetName(), Directory: e.GetDirectory(), Executable: e.GetExecutable(), Symlink: e.GetSymlink(), LinkTarget: e.GetLinkTarget()})
		}
		out.Truncated = r.GetTruncated()
		if emit != nil {
			emit(containerterm.PathListing{Entries: append([]containerterm.PathEntry(nil), out.Entries...), Truncated: out.Truncated})
		}
	}
}
