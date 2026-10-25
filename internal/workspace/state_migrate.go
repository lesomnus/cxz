package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/internal/dockerx"
)

// stateCache is the one thing in a project's state that is not worth moving.
// The agent's home lives in this volume, and most of what is in it is a build
// cache: across eleven projects, 126 of 152 gigabytes, and xli's 26GB of
// "project state" was 25GB of ~/.cache/go-build -- fifty thousand files at
// twelve megabytes a second, which is an hour of a project being down to move
// something that is regenerated on use by definition. It stays behind, and the
// hour and the disk stay with it. The first build in a migrated session is
// slower, once. See cxz#259.
const stateCache = "*/home/.cache"

// stateMovable reports whether this is the occasion to move a project's state:
// a recreate with no container of ours left, which is the only moment nothing
// is writing it. The first version asked this inside the branch that takes a
// container down, so a project whose container was already gone -- recreated
// after a crash, or stopped on purpose -- had no down to hang the move on and
// kept its own volume however many times it was recreated.
// A project's state can move only while nothing is writing it, which is any
// moment it has no container. That used to be spelled "during a recreate",
// because a recreate was the only way to get there; removing the container is
// its own operation now. migrateProjectStateWith does nothing for a project
// whose state has already moved.
func stateMovable(p *Project) bool {
	return p.ContainerID == ""
}

// migrateProjectState moves a project that still has a volume of its own into a
// subdirectory of the manager's. It runs where a recreate has already removed
// the container, which is the only moment nothing is writing that volume.
//
// The copy is verified before anything is removed: the two trees are compared
// by the name and size of every file in them, caches aside, and the volume is
// only let go when they agree. A project whose migration fails keeps its volume
// and is recreated exactly as it was -- the data has not moved, so there is
// nothing to recover. See cxz#238.
func (m *Manager) migrateProjectState(ctx context.Context, p *Project) error {
	return m.migrateProjectStateWith(ctx, p, dockerx.Run)
}

func (m *Manager) migrateProjectStateWith(ctx context.Context, p *Project, run removalDocker) error {
	if p.Volume == "" || m.StateVolume == "" {
		return nil
	}
	dir, _ := m.projectState(p.storageID())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	image := m.Image
	if image == "" {
		return fmt.Errorf("project %s state migration needs the manager image", p.ID)
	}
	// One container, both trees: the project's volume read-only, and the
	// manager's where this project's directory lives. tar rather than cp,
	// because tar can leave the caches behind.
	inside := "/to/projects/" + p.storageID() + "/state"
	if _, err := run(ctx, "run", "--rm",
		"--label", "cxz.owner="+m.Owner, "--label", "cxz.project="+p.storageID(),
		"-v", p.Volume+":/from:ro",
		"--mount", "type=volume,source="+m.StateVolume+",target=/to",
		"--entrypoint", "sh", image, "-c",
		"set -e; mkdir -p "+inside+
			"; tar -C /from --exclude='"+stateCache+"' -cf - . | tar -C "+inside+" -xf -"+
			// And out of the destination, for a migration being retried: an
			// interrupted first attempt can have copied gigabytes of cache that
			// nothing will read again, and nothing else would remove it.
			"; find "+inside+" -path '"+stateCache+"' -type d -prune -exec rm -rf {} +"); err != nil {
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
// sorted, with the caches pruned on both sides. It is what the migration
// compares: the copy has to be the same files with the same bytes, and a copy
// exiting zero is not that -- all the more so now that the copy is a pipeline,
// where a failure halfway through can still end in a shell that succeeded.
func (m *Manager) treeDigest(ctx context.Context, run removalDocker, image string, p *Project, path string) (string, error) {
	mounts := []string{"-v", p.Volume + ":/from:ro"}
	if strings.HasPrefix(path, "/to") {
		mounts = []string{"--mount", "type=volume,source=" + m.StateVolume + ",target=/to,readonly"}
	}
	args := append([]string{"run", "--rm", "--label", "cxz.owner=" + m.Owner, "--label", "cxz.project=" + p.storageID()}, mounts...)
	args = append(args, "--entrypoint", "sh", image, "-c",
		// The same exclusion on both sides, or the comparison would refuse
		// every migration for the caches it deliberately did not move.
		"cd "+path+" && find . -path '"+stateCache+"' -prune -o -type f -printf '%s %p\\n' | LC_ALL=C sort")
	b, err := run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("inventory %s of project %s: %w", path, p.ID, err)
	}
	return string(b), nil
}

// stateMount is where a project's state is, for a helper container that needs
// it mounted. One place knows the answer, because the answer differs per
// project: a project registered before the move still has a volume, and one
// after it is a subdirectory of the manager's.
func (m *Manager) stateMount(p *Project, target string, write bool) memoryMount {
	if p.Volume != "" {
		return memoryMount{volume: p.Volume, project: p.storageID(), target: target, write: write}
	}
	_, subpath := m.projectState(p.storageID())
	// The manager's volume carries the owner label and no project label, which
	// is what the ownership check compares against; the project is named by the
	// subpath rather than by a label of its own.
	return memoryMount{volume: m.StateVolume, target: target, subpath: subpath, write: write}
}

// stateLocation is how a project's state is named to a client. It stays a
// volume either way, because it is one.
func (m *Manager) stateLocation(p *Project) string {
	if p.Volume != "" {
		return "volume:" + p.Volume
	}
	_, subpath := m.projectState(p.storageID())
	return "volume:" + m.StateVolume + "/" + subpath
}

// readRuntime is the project's runtime.json as it stands, or empty when there
// is none. A project whose state is a subdirectory of the manager's volume is
// read directly; one that still has a volume is read through a container,
// because only a container can see inside a volume.
func (m *Manager) readRuntime(ctx context.Context, p *Project) ([]byte, error) {
	if p.Volume == "" {
		dir, _ := m.projectState(p.storageID())
		b, err := os.ReadFile(filepath.Join(dir, "runtime.json"))
		if os.IsNotExist(err) {
			return nil, nil
		}
		return b, err
	}
	return dockerx.Run(ctx, "run", "--rm", "--label", "cxz.owner="+m.Owner, "--label", "cxz.project="+p.storageID(),
		"--entrypoint", "sh", "--mount", "type=volume,source="+p.Volume+",target=/cxz/state,readonly", m.Image,
		"-c", "if test -f /cxz/state/runtime.json; then cat /cxz/state/runtime.json; fi")
}
