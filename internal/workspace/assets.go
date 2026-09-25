package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/dockerx"
)

// An upload queries only its target runtime instead of refreshing every session.
func (m *Manager) AttachmentSession(ctx context.Context, id string) (*api.Session, error) {
	projects, err := m.all(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		for _, session := range p.Sessions {
			if session.Id != id {
				continue
			}
			conn, client, err := m.client(ctx, p)
			if err != nil {
				return nil, err
			}
			defer conn.Close()
			v, err := client.Get(ctx, &api.SessionRef{Id: id})
			if err != nil {
				return nil, err
			}
			v.ProjectId = p.ID
			return v, nil
		}
	}
	return nil, fmt.Errorf("attachment session not found")
}

func (m *Manager) prepareAssetRoot(ctx context.Context, p *Project) (string, error) {
	if !assets.ValidID(p.ID) {
		return "", fmt.Errorf("invalid project ID")
	}
	local := assets.ExportRoot(filepath.Join(m.Root, "assets"), p.ID)
	if err := os.MkdirAll(local, 0755); err != nil {
		return "", err
	}
	manager, err := dockerx.Inspect(ctx, m.Container)
	if err != nil {
		return "", err
	}
	return assetEnginePath(local, manager)
}

// Docker bind sources live in the engine host's namespace, not the manager
// container's. Both named volumes and binds have an engine-visible Source.
func assetEnginePath(local string, manager dockerx.Container) (string, error) {
	local = filepath.Clean(local)
	best, source := "", ""
	for _, mount := range manager.Mounts {
		dest := filepath.Clean(mount.Destination)
		if (local == dest || strings.HasPrefix(local, dest+string(filepath.Separator))) && len(dest) > len(best) {
			best, source = dest, mount.Source
		}
	}
	if source == "" {
		return "", fmt.Errorf("asset directory has no engine-visible mount")
	}
	rel, err := filepath.Rel(best, local)
	if err != nil {
		return "", err
	}
	return filepath.Join(source, rel), nil
}

func (m *Manager) CheckAssetMount(ctx context.Context, project string) error {
	projects, err := m.all(ctx)
	if err != nil {
		return err
	}
	for _, p := range projects {
		if p.ID != project {
			continue
		}
		source, err := m.prepareAssetRoot(ctx, p)
		if err != nil {
			return err
		}
		container, err := dockerx.Inspect(ctx, p.ContainerID)
		if err != nil {
			return err
		}
		for _, mount := range container.Mounts {
			if mount.Destination == assets.MountPath && filepath.Clean(mount.Source) == source && !mount.RW {
				return nil
			}
		}
		return fmt.Errorf("project has no read-only asset mount; run cxz project recreate %s", p.Workspace)
	}
	return fmt.Errorf("attachment project not found")
}
