package workspace

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lesomnus/cxz/internal/dockerx"
)

// migrateProjectState moves a project that still has a volume of its own into a
// subdirectory of the manager's. It runs where a recreate has already removed
// the container, which is the only moment nothing is writing that volume.
//
// The copy is verified before anything is removed: the two trees are compared
// by the name and size of every file in them, and the volume is only let go
// when they agree. A project whose migration fails keeps its volume and is
// recreated exactly as it was -- the data has not moved, so there is nothing to
// recover. See cxz#238.
func (m *Manager) migrateProjectState(ctx context.Context, p *Project) error {
	return m.migrateProjectStateWith(ctx, p, dockerx.Run)
}

func (m *Manager) migrateProjectStateWith(ctx context.Context, p *Project, run removalDocker) error {
	if p.Volume == "" || m.StateVolume == "" {
		return nil
	}
	dir, _ := m.projectState(p.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	image := m.Image
	if image == "" {
		return fmt.Errorf("project %s state migration needs the manager image", p.ID)
	}
	// One container, both trees: the project's volume read-only, and the
	// manager's where this project's directory lives.
	inside := "/to/projects/" + p.ID + "/state"
	if _, err := run(ctx, "run", "--rm",
		"--label", "cxz.owner="+m.Owner, "--label", "cxz.project="+p.ID,
		"-v", p.Volume+":/from:ro",
		"--mount", "type=volume,source="+m.StateVolume+",target=/to",
		"--entrypoint", "sh", image, "-c",
		"set -e; mkdir -p "+inside+"; cp -a /from/. "+inside+"/"); err != nil {
		return fmt.Errorf("copy project %s state: %w", p.ID, err)
	}
	from, err := m.treeDigest(ctx, run, image, p, "/from")
	if err != nil {
		return err
	}
	to, err := m.treeDigest(ctx, run, image, p, inside)
	if err != nil {
		return err
	}
	if from != to {
		return fmt.Errorf("project %s state copy does not match its volume; the volume is untouched", p.ID)
	}
	previous := p.Volume
	p.Volume = ""
	if err = m.save(ctx, p); err != nil {
		p.Volume = previous
		return err
	}
	// The volume is only removed once the project no longer refers to it, so an
	// interruption leaves a copy to be remade rather than a project pointing at
	// something that is gone.
	if _, err = run(ctx, "volume", "rm", previous); err != nil {
		fmt.Fprintf(os.Stderr, "project %s state moved; its old volume %s could not be removed: %v\n", p.ID, previous, err)
	}
	fmt.Fprintf(os.Stderr, "project %s state moved into the manager's volume at projects/%s/state\n", p.ID, p.ID)
	return nil
}

// treeDigest is every file in a tree by name and size, in one line per file,
// sorted. It is what the migration compares: the copy has to be the same files
// with the same bytes, and `cp -a` exiting zero is not that.
func (m *Manager) treeDigest(ctx context.Context, run removalDocker, image string, p *Project, path string) (string, error) {
	mounts := []string{"-v", p.Volume + ":/from:ro"}
	if strings.HasPrefix(path, "/to") {
		mounts = []string{"--mount", "type=volume,source=" + m.StateVolume + ",target=/to,readonly"}
	}
	args := append([]string{"run", "--rm", "--label", "cxz.owner=" + m.Owner, "--label", "cxz.project=" + p.ID}, mounts...)
	args = append(args, "--entrypoint", "sh", image, "-c",
		"cd "+path+" && find . -type f -printf '%s %p\\n' | LC_ALL=C sort")
	b, err := run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("inventory %s of project %s: %w", path, p.ID, err)
	}
	return string(b), nil
}
