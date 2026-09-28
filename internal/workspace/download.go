package workspace

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/dockerx"
)

func (m *Manager) Download(ctx context.Context, project, path string, dst io.Writer) error {
	p, err := m.resolve(ctx, project)
	if err != nil {
		return err
	}
	if _, err = dockerx.Owned(ctx, p.ContainerID, m.Owner, p.ID); err != nil {
		return err
	}
	return containerterm.DownloadFile(ctx, projectView(p), path, dst)
}
