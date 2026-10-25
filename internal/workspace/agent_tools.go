package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/distribution"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/projectid"
)

func (r *Runtime) agentPath(kind string) (*string, error) {
	switch kind {
	case "claude":
		return &r.Claude, nil
	case "codex":
		return &r.Codex, nil
	default:
		return nil, fmt.Errorf("agent must be claude or codex")
	}
}

func (m *Manager) projectRuntime(ctx context.Context, p *Project) (Runtime, error) {
	raw, err := m.readRuntime(ctx, p)
	if err != nil {
		return Runtime{}, err
	}
	var r Runtime
	if len(raw) == 0 {
		return r, fmt.Errorf("project runtime manifest missing; run cxz project up")
	}
	if err = json.Unmarshal(raw, &r); err != nil {
		return r, fmt.Errorf("invalid project runtime manifest: %w", err)
	}
	if !projectid.Same(r.ProjectID, p.ID) || r.Token != p.Token {
		return r, fmt.Errorf("project runtime identity mismatch")
	}
	return r, nil
}

func (m *Manager) ensureAgentBinary(ctx context.Context, p *Project, kind string) (string, error) {
	arch, err := dockerx.Run(ctx, "exec", p.ContainerID, "uname", "-m")
	if err != nil {
		return "", err
	}
	libc, err := dockerx.Run(ctx, "exec", p.ContainerID, "sh", "-c", "ls /lib/ld-musl-*.so.1 2>/dev/null || true")
	if err != nil {
		return "", err
	}
	return distribution.Ensure(ctx, "/cxz/tools", kind, strings.TrimSpace(string(arch)), len(strings.TrimSpace(string(libc))) > 0)
}

func (m *Manager) verifyAgentBinary(ctx context.Context, p *Project, binary string) error {
	if binary == "" {
		return fmt.Errorf("agent executable path is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	_, err := dockerx.Run(ctx, "exec", "--user", p.RemoteUser, p.ContainerID, binary, "--version")
	return err
}

type agentPreparation struct {
	ensure func(context.Context, string) (string, error)
	verify func(context.Context, string) error
}

// prepareAgent changes only the default for future sessions. Every caller holds
// the project lock; no runtime, supervisor or existing agent is restarted.
func (m *Manager) prepareAgent(ctx context.Context, p *Project, kind string) error {
	return m.prepareAgentWith(ctx, p, kind, agentPreparation{
		ensure: func(ctx context.Context, kind string) (string, error) {
			return m.ensureAgentBinary(ctx, p, kind)
		},
		verify: func(ctx context.Context, binary string) error {
			return m.verifyAgentBinary(ctx, p, binary)
		},
	})
}

func (m *Manager) prepareAgentWith(ctx context.Context, p *Project, kind string, a agentPreparation) error {
	r, err := m.projectRuntime(ctx, p)
	if err != nil {
		return err
	}
	path, err := r.agentPath(kind)
	if err != nil {
		return err
	}
	binary, err := a.ensure(ctx, kind)
	if err == nil {
		err = a.verify(ctx, binary)
	}
	if err != nil {
		// An unavailable release must not strand a project with a working CLI.
		// Keep the registered choice only if the container can still execute it.
		if ctx.Err() == nil && *path != "" && *path != binary && a.verify(ctx, *path) == nil {
			fmt.Fprintln(os.Stderr, "agent preparation: using existing", kind, "executable:", err)
			return nil
		}
		return fmt.Errorf("prepare %s CLI: %w", kind, err)
	}
	if *path == binary {
		return nil
	}
	*path = binary
	return m.writeRuntime(ctx, p, r)
}

// promoteAgentDefault is called only with a confirmed update result (or a
// subsequent observation of the selected binary already running). A rollback
// never changes the project's choice for future sessions. Caller holds its lock.
func (m *Manager) promoteAgentDefault(ctx context.Context, p *Project, kind string, result *api.AgentUpdateStatus) error {
	if result == nil || result.State != "updated" || result.Binary == "" {
		return nil
	}
	if !distribution.ValidVersion(binaryVersion(kind, result.Binary)) {
		return fmt.Errorf("updated agent path is not a managed %s release", kind)
	}
	r, err := m.projectRuntime(ctx, p)
	if err != nil {
		return err
	}
	path, err := r.agentPath(kind)
	if err != nil {
		return err
	}
	if *path == result.Binary || newerVersion(binaryVersion(kind, *path), binaryVersion(kind, result.Binary)) {
		return nil
	}
	*path = result.Binary
	return m.writeRuntime(ctx, p, r)
}
