// Package purge deletes only inventoried resources owned by one cxz installation.
package purge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/transport"
)

type Docker func(context.Context, ...string) ([]byte, error)
type Target struct {
	Kind, ID, Name, Group, Stamp string
	Info                         os.FileInfo
}
type Group struct{ ID, Label string }

var Groups = []Group{
	{"containers", "Containers (manager, agents and writable layers)"},
	{"projects", "Project volumes (conversations and project login credentials)"},
	{"state", "Manager state volumes (database, accounts and tokens)"},
	{"tools", "Tool / other owned volumes"},
	{"networks", "Owned Docker networks"},
	{"local", "Local cxz state (settings, credentials, logs and installation locator)"},
}

type Plan struct {
	Root, Owner string
	Targets     []Target
	Preserved   []string
}

var localNames = map[string]bool{
	"file-mappings.json": true, "installation.json": true, "settings.json": true, "settings.jsonm": true, "settings.jsonc": true, "settings.schema.json": true, "cxz.db": true, "cxz.db-wal": true, "cxz.db-shm": true,
	"resources.db": true, "resources.db-wal": true, "resources.db-shm": true,
	"sessions": true, "projects": true, "run": true, "accounts": true, "central": true,
	"daemon.lock": true, "install.lock": true,
}

func safeRoot(root string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	for _, bad := range []string{"/", "/workspace", "/workspaces", "/tmp", "/var", "/var/lib", home, cwd} {
		if root == bad || (bad != "" && strings.HasPrefix(bad, root+string(os.PathSeparator))) {
			return "", fmt.Errorf("refusing broad state path %s", root)
		}
	}
	canonical, err := filepath.EvalSymlinks(root)
	if os.IsNotExist(err) {
		return root, nil
	}
	if err != nil {
		return "", err
	}
	if canonical != root {
		return "", fmt.Errorf("state path must not traverse symlinks")
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		return "", fmt.Errorf("refusing repository root")
	}
	return root, nil
}

func Discover(ctx context.Context, root string, d Docker) (Plan, error) {
	if d == nil {
		d = dockerx.Run
	}
	root, err := safeRoot(root)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{Root: root}
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		return p, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !localNames[name] && !strings.HasPrefix(name, ".cxz-write-") {
			p.Preserved = append(p.Preserved, filepath.Join(root, name))
			continue
		}
		info, err := os.Lstat(filepath.Join(root, name))
		if err != nil {
			return p, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return p, fmt.Errorf("refusing symlink state entry %s", name)
		}
		p.Targets = append(p.Targets, Target{Kind: "local", Name: filepath.Join(root, name), Group: "local", Info: info})
	}
	v, err := transport.Load(root)
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if !regexp.MustCompile(`^[a-f0-9]{24}$`).MatchString(v.Owner) {
		return p, fmt.Errorf("invalid installation owner; refusing broad Docker cleanup")
	}
	p.Owner = v.Owner
	for _, kind := range []string{"container", "volume", "network"} {
		args := []string{kind, "ls", "-q", "--filter", "label=cxz.owner=" + p.Owner}
		if kind == "container" {
			args = []string{"ps", "-aq", "--filter", "label=cxz.owner=" + p.Owner}
		}
		raw, err := d(ctx, args...)
		if err != nil {
			return p, err
		}
		for _, id := range strings.Fields(string(raw)) {
			t, err := inspect(ctx, d, kind, id, p.Owner)
			if err != nil {
				return p, err
			}
			switch kind {
			case "container":
				t.Group = "containers"
			case "network":
				t.Group = "networks"
			case "volume":
				switch {
				case t.Group != "":
					t.Group = "projects"
				case t.Name == v.StateVolume:
					t.Group = "state"
				default:
					t.Group = "tools"
				}
			}
			p.Targets = append(p.Targets, t)
		}
	}
	// Remove the manager before other containers, so it cannot restart projects.
	sort.SliceStable(p.Targets, func(i, j int) bool {
		rank := func(t Target) int {
			if t.Kind == "container" {
				if strings.TrimPrefix(t.Name, "/") == v.Container {
					return 0
				}
				return 1
			}
			if t.Kind == "network" {
				return 2
			}
			if t.Kind == "volume" {
				return 3
			}
			return 4
		}
		return rank(p.Targets[i]) < rank(p.Targets[j])
	})
	return p, nil
}

func inspect(ctx context.Context, d Docker, kind, id, owner string) (Target, error) {
	raw, err := d(ctx, kind, "inspect", id)
	if err != nil {
		return Target{}, err
	}
	var v []struct {
		Id, Name, Created, CreatedAt string
		Labels                       map[string]string
		Config                       struct{ Labels map[string]string }
	}
	if err = json.Unmarshal(raw, &v); err != nil || len(v) != 1 {
		return Target{}, fmt.Errorf("invalid %s inspection", kind)
	}
	labels := v[0].Labels
	if kind == "container" {
		labels = v[0].Config.Labels
	}
	if labels["cxz.owner"] != owner {
		return Target{}, fmt.Errorf("refusing unowned %s %s", kind, id)
	}
	if kind == "volume" {
		id = v[0].Name
	} else {
		id = v[0].Id
	}
	if id == "" || strings.HasPrefix(id, "-") {
		return Target{}, fmt.Errorf("invalid resource identity")
	}
	return Target{Kind: kind, ID: id, Name: v[0].Name, Stamp: v[0].Created + v[0].CreatedAt, Group: labels["cxz.project"]}, nil
}

func (p Plan) Validate(selected map[string]bool) error {
	count := 0
	for _, t := range p.Targets {
		if selected[t.Group] {
			count++
		}
	}
	if count == 0 {
		return fmt.Errorf("nothing selected")
	}
	for _, t := range p.Targets {
		if t.Kind == "container" && !selected["containers"] {
			for _, g := range []string{"projects", "state", "tools", "networks", "local"} {
				if selected[g] {
					return fmt.Errorf("select containers before deleting data, or keep all data categories")
				}
			}
		}
		if t.Kind != "local" && !selected[t.Group] && selected["local"] {
			return fmt.Errorf("keep local state while retaining Docker resources, so their installation identity is not lost")
		}
	}
	return nil
}

// Execute requires the caller's explicit confirmation. Never invoked by discovery.
func Execute(ctx context.Context, p Plan, selected map[string]bool, d Docker, out io.Writer) error {
	if d == nil {
		d = dockerx.Run
	}
	if err := p.Validate(selected); err != nil {
		return err
	}
	if _, err := safeRoot(p.Root); err != nil {
		return err
	}
	// Serialize installation; refuse active native daemons/supervisors.
	var locks []*os.File
	createdLocks := map[string]os.FileInfo{}
	defer func() {
		for _, f := range locks {
			f.Close()
		}
		for path, original := range createdLocks {
			if current, err := os.Lstat(path); err == nil && os.SameFile(original, current) {
				_ = os.Remove(path)
			}
		}
	}()
	for _, name := range []string{"install.lock", "daemon.lock"} {
		if _, err := os.Stat(p.Root); os.IsNotExist(err) {
			break
		}
		path := filepath.Join(p.Root, name)
		_, before := os.Lstat(path)
		f, err := core.Lock(path)
		if err != nil {
			return fmt.Errorf("stop local cxz processes first: %w", err)
		}
		locks = append(locks, f)
		if os.IsNotExist(before) {
			info, err := f.Stat()
			if err != nil {
				return err
			}
			createdLocks[path] = info
		}
	}
	for _, t := range p.Targets {
		if !selected[t.Group] {
			continue
		}
		if t.Kind == "local" {
			err := filepath.WalkDir(t.Name, func(path string, e os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if e.Type()&os.ModeSymlink != 0 {
					return nil
				}
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".lock") && e.Name() != "install.lock" && e.Name() != "daemon.lock" {
					f, err := core.Lock(path)
					if err != nil {
						return err
					}
					locks = append(locks, f)
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("stop local cxz processes first: %w", err)
			}
		}
	}
	// Revalidate the full inventory before the first deletion.
	current, err := Discover(ctx, p.Root, d)
	if err != nil {
		return err
	}
	if current.Owner != p.Owner || !sameTargets(p.Targets, current.Targets) {
		return fmt.Errorf("cleanup inventory changed; run purge again")
	}
	for _, t := range p.Targets {
		if !selected[t.Group] || t.Kind == "local" {
			continue
		}
		actual, err := inspect(ctx, d, t.Kind, t.ID, p.Owner)
		if err != nil {
			return err
		}
		if actual.ID != t.ID || actual.Stamp != t.Stamp {
			return fmt.Errorf("resource changed: %s", t.Name)
		}
		args := []string{t.Kind, "rm", t.ID}
		if t.Kind == "container" {
			args = []string{"rm", "-f", t.ID}
		}
		if _, err = d(ctx, args...); err != nil {
			return fmt.Errorf("cleanup stopped; locator retained: %w", err)
		}
		fmt.Fprintf(out, "Deleted %s %s (not recoverable without backup)\n", t.Kind, t.Name)
	}
	if !selected["local"] {
		return nil
	}
	remaining, err := Discover(ctx, p.Root, d)
	if err != nil {
		return err
	}
	for _, t := range remaining.Targets {
		if t.Kind != "local" {
			return fmt.Errorf("new Docker resources appeared; local locator retained, run purge again")
		}
	}
	// Individual validated children only: never recursively remove the state root.
	for _, t := range p.Targets {
		if t.Kind != "local" || filepath.Base(t.Name) == "installation.json" {
			continue
		}
		if err := removeLocal(p.Root, t); err != nil {
			return err
		}
		fmt.Fprintf(out, "Deleted %s (not recoverable without backup)\n", t.Name)
	}
	for _, t := range p.Targets {
		if t.Kind == "local" && filepath.Base(t.Name) == "installation.json" {
			if err := removeLocal(p.Root, t); err != nil {
				return err
			}
			fmt.Fprintln(out, "Deleted", t.Name)
		}
	}
	return nil
}
func sameTargets(a, b []Target) bool {
	// Lock files may be created when taking the cleanup locks.
	key := func(t Target) string { return t.Kind + "|" + t.ID + "|" + t.Name + "|" + t.Stamp }
	expected := map[string]bool{}
	localInfo := map[string]os.FileInfo{}
	for _, t := range a {
		expected[key(t)] = true
		if t.Kind == "local" {
			localInfo[t.Name] = t.Info
		}
	}
	for _, t := range b {
		if t.Kind == "local" && (filepath.Base(t.Name) == "install.lock" || filepath.Base(t.Name) == "daemon.lock") {
			delete(expected, key(t))
			continue
		}
		if !expected[key(t)] {
			return false
		}
		if t.Kind == "local" && !os.SameFile(localInfo[t.Name], t.Info) {
			return false
		}
		delete(expected, key(t))
	}
	return len(expected) == 0
}
func removeLocal(root string, t Target) error {
	if filepath.Dir(t.Name) != root {
		return fmt.Errorf("invalid local target")
	}
	actual, err := os.Lstat(t.Name)
	if err != nil {
		return err
	}
	if actual.Mode()&os.ModeSymlink != 0 || !os.SameFile(t.Info, actual) {
		return fmt.Errorf("local target replaced: %s", t.Name)
	}
	return os.RemoveAll(t.Name)
}
