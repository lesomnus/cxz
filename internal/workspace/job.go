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
		if c.Config.Labels["cxz.owner"] != m.Owner || c.Config.Labels["cxz.project"] != p.storageID() || c.Config.Labels["devcontainer.local_folder"] != p.Workspace {
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

// settle retires a recorded provisioning failure the runtime has since
// disproved, and reports whether anything changed.
//
// `Error` and a failed job describe one attempt, but the project page reports
// them as the project's present state. After cxz#280 this repository's own
// project showed `ping: unable to open database file` for an hour with every
// call succeeding again, because nothing but another `up` ever clears the
// record.
//
// A runtime that answers proves the container runs, the network is attached,
// the token works and the runtime serves -- every step up to `runtime-ready`.
// `session` is the only step after it, so a failure there is still true: the
// session the caller asked for does not exist. That one stays.
func (p *Project) settle() bool {
	if p.Error == "" && p.Job.State != "failed" && p.Job.State != "interrupted" {
		return false
	}
	if p.Job.Step == "session" {
		return false
	}
	p.Error = ""
	if p.Job.State != "complete" {
		p.Job.State, p.Job.Step = "complete", "ready"
		p.Job.UpdatedAt = time.Now().UnixMilli()
	}
	return true
}
