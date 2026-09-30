// Package sessionpurge destroys what one session left on disk.
//
// Deleting a session is a tombstone: the record stops being listed, but the
// journal, the agent profile and the uploads stay behind, so a mistaken delete
// stays recoverable. Purge is the deliberate opposite, and the only code in
// cxz that unlinks a journal. It takes the same inventory twice -- once to
// report, once to delete -- so a dry run shows the caller the exact bytes they
// are about to lose rather than a promise about them.
package sessionpurge

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/memorylib"
)

// MaxSpec bounds a purge request the way every other Docker action spec is
// bounded: a purge carries one handle, never a payload.
const MaxSpec = 4096

// Session and creation ids are 24 hex characters. Refusing anything else is
// what stops a crafted spec from turning a path join into a traversal.
var validID = regexp.MustCompile(`^[a-f0-9]{24}$`)

type Request struct {
	Session string `json:"session"`
	DryRun  bool   `json:"dry_run,omitempty"`
}

// Subject is a session already resolved to the identifiers its files are named
// after. CreateID and Project are empty on a session that never launched; the
// paths that need them are then simply not in the plan.
type Subject struct {
	Session  string `json:"session"`
	CreateID string `json:"create_id,omitempty"`
	Project  string `json:"project,omitempty"`
}

func (s Subject) validate() error {
	if !validID.MatchString(s.Session) {
		return fmt.Errorf("invalid session id")
	}
	if s.CreateID != "" && !validID.MatchString(s.CreateID) {
		return fmt.Errorf("invalid session creation key")
	}
	if s.Project != "" && !assets.ValidID(s.Project) {
		return fmt.Errorf("invalid project id")
	}
	return nil
}

// Target is one thing purge found, with what deleting it costs.
type Target struct {
	Kind  string `json:"kind"`
	Path  string `json:"path"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
}

type Reply struct {
	Session string   `json:"session"`
	DryRun  bool     `json:"dry_run,omitempty"`
	Targets []Target `json:"targets,omitempty"`
	// Retained names what purge deliberately left, so the caller can say so
	// instead of implying the session vanished from the world.
	Retained []string `json:"retained,omitempty"`
}

func (r Reply) Bytes() int64 {
	var n int64
	for _, t := range r.Targets {
		n += t.Bytes
	}
	return n
}

// Scope names which state root a purge is running against. A session's data is
// split across two: the conversation lives inside the project runtime that ran
// it, while the uploads live on the manager host because they have to be
// reachable from outside the project to be bind-mounted back into it.
type Scope string

const (
	Runtime Scope = "runtime"
	Manager Scope = "manager"
)

// Paths is the deletion order, and the order is the crash plan. The session
// manifest goes last because the server rebuilds its database from the
// manifests on disk: a purge interrupted halfway leaves a session that still
// exists and can be purged again, never a live session pointing at a journal
// that is already gone.
func Paths(scope Scope, root string, s Subject) ([]Target, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var out []Target
	add := func(kind, path string) {
		out = append(out, Target{Kind: kind, Path: path})
	}
	if scope == Manager {
		if s.Project != "" {
			add("uploads", filepath.Join(assets.ExportRoot(filepath.Join(root, "assets"), s.Project), s.Session))
		}
	} else {
		if s.Project != "" {
			add("memory", filepath.Join(memorylib.Dir(root, s.Project), memorylib.SnapshotID(s.Session)))
		}
		if s.CreateID != "" {
			add("profile", accounts.SessionRoot(root, s.CreateID))
		}
		add("socket", core.Socket(root, s.Session))
		add("journal", core.Dir(root, s.Session))
	}
	for _, t := range out {
		if err := contained(root, t.Path); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// contained rejects a path that is not strictly inside root. Every path here is
// built from a validated id, so a failure means a bug rather than an attack --
// which is exactly why it must not be silent.
func contained(root, path string) error {
	clean := filepath.Clean(path)
	if clean == root || !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
		return fmt.Errorf("purge target %s escapes %s", path, root)
	}
	if rest := strings.TrimPrefix(clean, root+string(os.PathSeparator)); strings.Count(rest, string(os.PathSeparator)) < 1 {
		return fmt.Errorf("purge target %s is a top-level state directory", path)
	}
	return nil
}

// Plan reports without touching anything.
func Plan(ctx context.Context, scope Scope, root string, s Subject) (Reply, error) {
	return run(ctx, scope, root, s, true)
}

// Execute deletes, and reports what it actually deleted. An absent target is not
// an error: purge is re-runnable by design, and a session that never uploaded a
// file has no uploads to lose.
func Execute(ctx context.Context, scope Scope, root string, s Subject) (Reply, error) {
	return run(ctx, scope, root, s, false)
}

func run(ctx context.Context, scope Scope, root string, s Subject, dry bool) (Reply, error) {
	targets, err := Paths(scope, root, s)
	if err != nil {
		return Reply{}, err
	}
	out := Reply{Session: s.Session, DryRun: dry}
	for _, t := range targets {
		files, bytes, found, err := measure(t.Path)
		if err != nil {
			return out, err
		}
		if !found {
			continue
		}
		t.Files, t.Bytes = files, bytes
		if !dry {
			if err := erase(ctx, root, s, t); err != nil {
				return out, err
			}
		}
		out.Targets = append(out.Targets, t)
	}
	return out, nil
}

// erase unlinks a target. Uploads are the one kind that is not just a directory:
// each published file is a hard link into a content-addressed store other
// sessions may share, so removing the directory alone would leak the bytes.
func erase(ctx context.Context, root string, s Subject, t Target) error {
	if t.Kind == "uploads" {
		return assets.Forget(ctx, filepath.Join(root, "assets"), s.Project, s.Session)
	}
	return os.RemoveAll(t.Path)
}

// measure refuses a symlinked target instead of following it out of the state
// directory, and counts only the bytes purge is responsible for.
func measure(path string) (int, int64, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, 0, false, fmt.Errorf("refusing symlinked purge target %s", path)
	}
	if !info.IsDir() {
		return 1, info.Size(), true, nil
	}
	files := 0
	var bytes int64
	err = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		files++
		bytes += info.Size()
		return nil
	})
	return files, bytes, true, err
}
