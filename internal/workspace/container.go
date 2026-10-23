package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The container operations a project's lifecycle is made of, separate from the
// registration that owns them. A project is a registered workspace; its
// container is a thing that can be created, stopped, started and thrown away
// without the project, its sessions or their conversations going anywhere.
// See docs/plans/project-lifecycle-api.md.
//
// The boundary follows the Dev Container lifecycle rather than Docker's, which
// is why creating leaves a container running: `onCreateCommand` and
// `postCreateCommand` run inside the container, so a created-but-never-run
// container could only be produced by starting it and stopping it again. The
// CLI keeps the two sets of hooks apart by itself -- measured in cxz#274 --
// and the start path asks it for an existing container so it cannot do more.

// Precondition is a state the caller has to change before the operation makes
// sense. It is not a failure of the operation.
type Precondition struct{ Message string }

func (e *Precondition) Error() string { return e.Message }
func (e *Precondition) GRPCStatus() *status.Status {
	return status.New(codes.FailedPrecondition, e.Message)
}

// StopContainer leaves the container where it is. The agents are asked to exit
// first, so a session that comes back is a session that stopped rather than one
// that was killed.
func (m *Manager) stopContainer(ctx context.Context, p *Project) (retErr error) {
	defer m.recordFailure(ctx, p, &retErr)
	m.quiesce(ctx, p)
	containers, err := m.owned(ctx, p)
	if err != nil {
		return err
	}
	for _, v := range containers {
		if !v.State.Running {
			continue
		}
		if _, err = dockerx.Run(ctx, "stop", "--time", "10", v.ID); err != nil {
			return err
		}
	}
	p.Error = ""
	p.Job.State, p.Job.Step = "complete", "stopped"
	p.Job.UpdatedAt = time.Now().UnixMilli()
	return m.save(ctx, p)
}

// StartContainer starts the container this project already has and boots its
// runtime. It never creates one: a project with nothing to start is a
// precondition for the caller, not a container to be conjured.
//
// A configuration that changed since the container was made is refused rather
// than applied, because it cannot be applied -- the Dev Container CLI reuses
// the container it finds and ignores the difference without a word. Applying it
// means replacing the container, which is the caller's decision to make.
func (m *Manager) startContainer(ctx context.Context, p *Project) (retErr error) {
	defer m.recordFailure(ctx, p, &retErr)
	containers, err := m.owned(ctx, p)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return &Precondition{Message: fmt.Sprintf("project %s has no container to start", p.ID)}
	}
	config, err := m.render(ctx, p)
	if err != nil {
		return err
	}
	if err = config.startable(p); err != nil {
		return err
	}
	if err = m.checkpoint(ctx, p, "start"); err != nil {
		return err
	}
	result, err := m.devcontainerUp(ctx, p, config.configFile, m.overrideConfig(p), true)
	if err != nil {
		return err
	}
	p.ContainerID = result.ContainerID
	if err = m.boot(ctx, p); err != nil {
		return err
	}
	p.ConfigDigest = config.trust.Digest()
	p.Error = ""
	p.Job.State, p.Job.Step = "complete", "ready"
	p.Job.UpdatedAt = time.Now().UnixMilli()
	return m.save(ctx, p)
}

// RemoveContainer throws the containers away and keeps everything else: the
// registration, the sessions, their conversations, the project's state volume
// and directory. What it removes is what a recreate would have removed anyway.
func (m *Manager) removeContainer(ctx context.Context, p *Project) (retErr error) {
	defer m.recordFailure(ctx, p, &retErr)
	m.quiesce(ctx, p)
	containers, err := m.owned(ctx, p)
	if err != nil {
		return err
	}
	for _, v := range containers {
		if _, err = dockerx.Run(ctx, "rm", "-f", v.ID); err != nil {
			return err
		}
	}
	p.ContainerID = ""
	p.Error = ""
	p.Job.State, p.Job.Step = "complete", "down"
	p.Job.UpdatedAt = time.Now().UnixMilli()
	return m.save(ctx, p)
}

// recordFailure leaves behind what went wrong, the way provisioning does, so a
// container operation that failed is visible rather than only absent from its
// result. A precondition is recorded too: it describes the project's state, and
// the next operation that succeeds retires it (cxz#282).
func (m *Manager) recordFailure(ctx context.Context, p *Project, err *error) {
	if *err == nil {
		return
	}
	p.Job.State = "failed"
	p.Job.UpdatedAt = time.Now().UnixMilli()
	p.Error = (*err).Error()
	if e := m.save(context.WithoutCancel(ctx), p); e != nil {
		*err = fmt.Errorf("%w; recording failure: %v", *err, e)
	}
}

// owned is every container this manager made for this project, checked for
// ownership so no operation here can touch something it did not create.
func (m *Manager) owned(ctx context.Context, p *Project) ([]dockerx.Container, error) {
	containers, err := dockerx.List(ctx, "label=cxz.owner="+m.Owner, "label=cxz.project="+p.ID)
	if err != nil {
		return nil, err
	}
	out := make([]dockerx.Container, 0, len(containers))
	for _, v := range containers {
		full, err := dockerx.Owned(ctx, v.ID, m.Owner, p.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, full)
	}
	return out, nil
}

// quiesce asks the agents to exit. Best effort: a project that cannot answer
// must still be stoppable and removable.
func (m *Manager) quiesce(ctx context.Context, p *Project) {
	conn, client, err := m.client(ctx, p)
	if err != nil {
		return
	}
	defer conn.Close()
	if err = m.stopSessions(ctx, p, client); err != nil {
		fmt.Fprintln(os.Stderr, "stopping sessions:", err)
	}
}

// rendered is the configuration a project would be provisioned with: the file
// the Dev Container CLI is pointed at, the Compose override cxz merges in, and
// the ledger of what all of it asks for -- which is also what identifies it,
// since the digest is taken over the same documents.
//
// Both paths ask for it. Creating acts on it; starting only compares it, so
// that a configuration which changed since the container was made is reported
// instead of being quietly left unapplied.
type rendered struct {
	configFile string
	override   Override
	trust      *trust
}

// startable reports why the configuration as it stands now cannot be applied
// to the container that already exists.
//
// A change is refused rather than applied because starting cannot apply it: the
// CLI reuses the container it finds. Applying means replacing the container,
// which costs the container's own filesystem and is therefore the caller's
// decision. A project with no digest recorded was provisioned before cxz kept
// one, so there is nothing to compare and nothing to refuse.
func (c rendered) startable(p *Project) error {
	if err := c.trust.decide(p, false); err != nil {
		return err
	}
	if p.ConfigDigest != "" && p.ConfigDigest != c.trust.Digest() {
		return &Precondition{Message: "configuration changed since this container was created; recreate the project to apply it"}
	}
	return nil
}

func (m *Manager) render(ctx context.Context, p *Project) (rendered, error) {
	var out rendered
	configFile, err := m.provisionConfiguration(p)
	if err != nil {
		return out, err
	}
	configured := *p
	configured.Config = configFile
	seen := &trust{}
	if err = preflight(&configured, seen); err != nil {
		return out, err
	}
	override, err := m.prepareComposeOverride(ctx, &configured, seen)
	if err != nil {
		return out, err
	}
	return rendered{configFile: configFile, override: override, trust: seen}, nil
}

// overrideConfig is the configuration cxz rendered for the container that
// exists, written when it was created. Starting applies what the container was
// made with; a change to the source configuration is refused above, not
// silently rendered into a file the CLI then ignores.
func (m *Manager) overrideConfig(p *Project) string {
	return filepath.Join(m.Root, "projects", p.ID, "devcontainer.json")
}

// containerState is derived from what was collected, every time it is asked,
// and is written down nowhere. A project resource exists whether or not a
// container does; what the container is doing is a fact about the container, so
// it is read from the container.
//
// `unknown` is the answer when Docker could not be asked. It used to be
// `absent`: the look and the absence shared a value, so a daemon hiccup read as
// "this project has no container" -- and `reconcile` would then clear the
// recorded container id to match. A conclusion stored in place of what was
// observed is the same mistake cxz#282 cost an hour of a wrong error on the
// project page.
func containerState(p *Project, inventory []dockerx.Container, lookErr error) string {
	if lookErr != nil {
		return "unknown"
	}
	for _, v := range inventory {
		// The labels, not the recorded id: the id is what cxz last acted on,
		// the labels are what the container says it is.
		if v.Config.Labels["cxz.project"] != p.ID || v.Config.Labels["devcontainer.local_folder"] != p.Workspace {
			continue
		}
		if v.State.Running {
			return "running"
		}
		return "stopped"
	}
	return "absent"
}

// The runtime API entry points. They resolve the project, take its lock and
// answer with a receipt, the way Open and Down do -- the operations above are
// what they run once the project is settled.

func (m *Manager) StopContainer(ctx context.Context, r *api.ProjectRequest) (*api.Receipt, error) {
	return m.containerOp(ctx, r, (*Manager).stopContainer, "stopped")
}

func (m *Manager) containerOp(ctx context.Context, r *api.ProjectRequest, op func(*Manager, context.Context, *Project) error, status string) (*api.Receipt, error) {
	p, err := m.resolve(ctx, r.Workspace)
	if err != nil {
		return nil, err
	}
	lock := m.projectLock(p.ID)
	lock.Lock()
	defer lock.Unlock()
	// Under the lock, because the project may have moved while it was waiting.
	if p, err = m.resolve(ctx, p.ID); err != nil {
		return nil, err
	}
	if err = op(m, ctx, p); err != nil {
		return nil, err
	}
	return &api.Receipt{ClientId: r.ClientId, Status: status}, nil
}
