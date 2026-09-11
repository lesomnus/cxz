package workspace

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/internal/dockerx"
	"time"
)

// Checkpoints are diagnostic, not permission to skip external effects. A retry
// rechecks ownership and converges resources before continuing. User hooks may
// execute again; user messages are never replayed by provisioning.
type ProvisionJob struct {
	State     string `json:"state"`
	Step      string `json:"step"`
	Attempt   uint64 `json:"attempt"`
	UpdatedAt int64  `json:"updated_at"`
}

func (m *Manager) reconcile(ctx context.Context, p *Project, containers []dockerx.Container) error {
	id := ""
	for _, c := range containers {
		if c.Config.Labels["cxz.owner"] != m.Owner || c.Config.Labels["cxz.project"] != p.ID || c.Config.Labels["devcontainer.local_folder"] != p.Workspace {
			continue
		}
		if id != "" {
			return fmt.Errorf("multiple owned workspace containers for %s; manual inspection required", p.ID)
		}
		id = c.ID
	}
	if id == p.ContainerID {
		return nil
	}
	p.ContainerID = id
	return m.save(ctx, p)
}

func (m *Manager) checkpoint(ctx context.Context, p *Project, step string) error {
	p.Job.State, p.Job.Step = "running", step
	p.Job.UpdatedAt = time.Now().UnixMilli()
	p.Error = ""
	return m.save(ctx, p)
}
