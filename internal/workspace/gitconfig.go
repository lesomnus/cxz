package workspace

import (
	"context"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/hostgit"
)

func (m *Manager) installGitConfig(ctx context.Context, p *Project) error {
	file, err := os.Open(filepath.Join(m.Root, hostgit.SnapshotFile))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	b, err := hostgit.Read(file)
	if err != nil {
		return err
	}
	return hostgit.Inject(ctx, p.ContainerID, p.RemoteUser, b)
}
