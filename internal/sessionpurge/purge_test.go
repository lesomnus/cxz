package sessionpurge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/memorylib"
)

const (
	session   = "4d683e9c0f064572059be61b"
	createKey = "9b3bc6ba06265a43c6fd9f26"
	other     = "1111111111111111111111aa"
	project   = "48f1747531aa5bbc5e6b94ac"
)

func subject() Subject {
	return Subject{Session: session, CreateID: createKey, Project: project}
}

// populate writes one file into every place a purge is supposed to reach, plus a
// neighbouring session's copy of each, and returns the path of each file.
func populate(t *testing.T, root string) (mine, theirs []string) {
	t.Helper()
	neighbour := Subject{Session: other, CreateID: other, Project: project}
	for _, s := range []Subject{subject(), neighbour} {
		files := []string{
			filepath.Join(core.Dir(root, s.Session), "events.jsonl"),
			core.Socket(root, s.Session),
			filepath.Join(accounts.SessionRoot(root, s.CreateID), "accounts", "work", "config", "transcript.jsonl"),
			filepath.Join(memorylib.Dir(root, s.Project), memorylib.SnapshotID(s.Session), "overview.md"),
			filepath.Join(assets.ExportRoot(filepath.Join(root, "assets"), s.Project), s.Session, "abc", "note.txt"),
		}
		for _, path := range files {
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("payload for "+s.Session), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if s.Session == session {
			mine = files
		} else {
			theirs = files
		}
	}
	return mine, theirs
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return err == nil
}

func TestPurgeReachesEveryPlaceASessionWrote(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	mine, theirs := populate(t, root)
	// The two scopes together are the whole session; neither alone is.
	var kinds []string
	for _, scope := range []Scope{Runtime, Manager} {
		reply, err := Execute(ctx, scope, root, subject())
		if err != nil {
			t.Fatal(scope, err)
		}
		for _, target := range reply.Targets {
			kinds = append(kinds, target.Kind)
		}
	}
	for _, want := range []string{"journal", "socket", "profile", "memory", "uploads"} {
		if !strings.Contains(strings.Join(kinds, " "), want) {
			t.Fatal("purge never reported", want, "in", kinds)
		}
	}
	for _, path := range mine {
		if exists(t, path) {
			t.Fatal("purge left", path)
		}
	}
	for _, path := range theirs {
		if !exists(t, path) {
			t.Fatal("purge took a neighbouring session's", path)
		}
	}
}

func TestPurgeReportsTheBytesADryRunPromised(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	mine, _ := populate(t, root)
	var want int64
	for _, path := range mine {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		want += info.Size()
	}
	var planned, deleted int64
	for _, scope := range []Scope{Runtime, Manager} {
		plan, err := Plan(ctx, scope, root, subject())
		if err != nil {
			t.Fatal(err)
		}
		planned += plan.Bytes()
		if !plan.DryRun {
			t.Fatal("a plan must report itself as a dry run")
		}
		done, err := Execute(ctx, scope, root, subject())
		if err != nil {
			t.Fatal(err)
		}
		deleted += done.Bytes()
	}
	if planned != want || deleted != want {
		t.Fatal("byte counts disagree: planned", planned, "deleted", deleted, "actual", want)
	}
	for _, path := range mine {
		if exists(t, path) {
			t.Fatal("a dry run deleted", path)
		}
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	mine, _ := populate(t, root)
	for _, scope := range []Scope{Runtime, Manager} {
		if _, err := Plan(ctx, scope, root, subject()); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range mine {
		if !exists(t, path) {
			t.Fatal("dry run deleted", path)
		}
	}
}

func TestPurgeIsRerunnable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	populate(t, root)
	for i := 0; i < 2; i++ {
		for _, scope := range []Scope{Runtime, Manager} {
			reply, err := Execute(ctx, scope, root, subject())
			if err != nil {
				t.Fatal("re-run", i, scope, err)
			}
			if i == 1 && len(reply.Targets) != 0 {
				t.Fatal("second run still found", reply.Targets)
			}
		}
	}
}

func TestMalformedSubjectsAreRefusedBeforeAnyDeletion(t *testing.T) {
	root := t.TempDir()
	for name, s := range map[string]Subject{
		"traversal":    {Session: "../../etc", Project: project},
		"empty":        {},
		"short":        {Session: "abc"},
		"bad create":   {Session: session, CreateID: "../.."},
		"bad project":  {Session: session, Project: "../../.."},
		"upper case":   {Session: strings.ToUpper(session)},
		"long session": {Session: session + session},
	} {
		if _, err := Paths(Runtime, root, s); err == nil {
			t.Fatal(name, "was accepted")
		}
	}
}

// The journal is the manifest the runtime rebuilds its database from, so it must
// be unlinked after everything else: a purge killed midway then leaves a session
// that still exists and can simply be purged again.
func TestJournalIsUnlinkedLast(t *testing.T) {
	targets, err := Paths(Runtime, t.TempDir(), subject())
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) == 0 || targets[len(targets)-1].Kind != "journal" {
		t.Fatal("journal is not the last target", targets)
	}
}

func TestSymlinkedTargetsAreRefusedRatherThanFollowed(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("not ours"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, core.Dir(root, session)); err != nil {
		t.Fatal(err)
	}
	if _, err := Execute(ctx, Runtime, root, subject()); err == nil {
		t.Fatal("followed a symlinked journal")
	}
	if !exists(t, filepath.Join(outside, "keep.txt")) {
		t.Fatal("purge deleted through a symlink")
	}
}

// A session that never launched has no profile and no project, and purging it
// must still work rather than failing on the paths it cannot name.
func TestSessionWithoutAProfileOrProject(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(core.Dir(root, session), "session.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	reply, err := Execute(ctx, Runtime, root, Subject{Session: session})
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Targets) != 1 || reply.Targets[0].Kind != "journal" {
		t.Fatal("unexpected targets", reply.Targets)
	}
	if exists(t, path) {
		t.Fatal("journal survived")
	}
}
