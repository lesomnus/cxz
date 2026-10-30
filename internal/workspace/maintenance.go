package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/distribution"
	"github.com/lesomnus/cxz/internal/maintenance"
)

func (m *Manager) maintenanceRoots(ctx context.Context) ([]string, error) {
	projects, err := m.all(ctx)
	if err != nil {
		return nil, err
	}
	roots := []string{m.Root}
	for _, p := range projects {
		if p.Volume != "" {
			return nil, fmt.Errorf("shared executable cleanup skipped: legacy project %s has an inaccessible state volume", p.ID)
		}
		state, _ := m.projectState(p.storageID())
		root := filepath.Join(state, "data")
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			return nil, fmt.Errorf("shared executable cleanup skipped: state unavailable for project %s", p.ID)
		}
		roots = append(roots, root)
	}
	return roots, nil
}

// Only the Manager can see the installation's complete reference set and write
// the shared tools mount. A legacy/inaccessible project prevents shared GC.
func (m *Manager) MaintenanceVersions(ctx context.Context, p maintenance.Policy, now time.Time, pressure bool) ([]maintenance.Candidate, error) {
	return m.maintenanceVersions(ctx, "/cxz/tools", p, now, pressure)
}
func (m *Manager) maintenanceVersions(ctx context.Context, tools string, p maintenance.Policy, now time.Time, pressure bool) ([]maintenance.Candidate, error) {
	references := func(ctx context.Context) (maintenance.References, error) {
		roots, err := m.maintenanceRoots(ctx)
		if err != nil {
			return maintenance.References{}, err
		}
		r, err := maintenance.ReadReferences(ctx, roots, tools)
		if err != nil {
			return r, err
		}
		for kind, fallback := range map[string]string{"claude": distribution.ClaudeVersion, "codex": distribution.CodexVersion, "gh": distribution.GitHubVersion} {
			r.Paths[filepath.Join(tools, kind, distribution.SelectedVersion(tools, kind, fallback))] = true
		}
		return r, nil
	}
	// Create the shared readable lease before the first collector pass.
	release, err := maintenance.VersionLease(ctx, tools)
	if err != nil {
		return nil, err
	}
	release()
	groups := []string{filepath.Join(tools, "claude"), filepath.Join(tools, "codex"), filepath.Join(tools, "gh"), filepath.Join(tools, "cxz-builds", "releases"), filepath.Join(tools, "use")}
	versions, err := maintenance.VersionCandidates(ctx, groups, maintenance.VersionLock(tools), references, p, now, pressure)
	if err != nil {
		return nil, err
	}
	staging, err := maintenance.StagingCandidates([]string{filepath.Join(tools, "claude", "*", ".download-*"), filepath.Join(tools, "codex", "*", ".download-*"), filepath.Join(tools, "gh", "*", ".gh-*"), filepath.Join(tools, ".cxz-*"), filepath.Join(tools, "cxz-builds", "releases", "*", "*", ".cxz-*"), filepath.Join(tools, "cxz-builds", "releases", "*", "*", "*", ".cxz-*")}, maintenance.VersionLock(tools), p, now, pressure)
	return append(versions, staging...), err
}

// Shared-state projects receive an explicitly configured Manager policy without
// adding a transport-specific maintenance RPC. Legacy volumes keep local policy.
func (m *Manager) SyncMaintenancePolicy(ctx context.Context) error {
	if _, err := os.Stat(filepath.Join(m.Root, "maintenance.json")); os.IsNotExist(err) {
		return nil
	}
	p, err := maintenance.Load(m.Root)
	if err != nil {
		return err
	}
	projects, err := m.all(ctx)
	if err != nil {
		return err
	}
	for _, project := range projects {
		if project.Volume != "" {
			continue
		}
		state, _ := m.projectState(project.storageID())
		root := filepath.Join(state, "data")
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if existing, err := maintenance.Load(root); err == nil && existing == p {
			continue
		}
		if err = core.WriteJSON(filepath.Join(root, "maintenance.json"), p); err != nil {
			return err
		}
		// Policy is non-secret and must be readable by the project's remote user
		// even when the Manager writes the shared volume as root.
		if err = os.Chmod(filepath.Join(root, "maintenance.json"), 0644); err != nil {
			return err
		}
	}
	return nil
}
