package maintenance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestVersionGCProtectsReferencesGraceRecentAndPublicationLease(t *testing.T) {
	root := t.TempDir()
	group := filepath.Join(root, "codex")
	os.MkdirAll(group, 0755)
	now := time.Now()
	for i := 1; i <= 6; i++ {
		path := filepath.Join(group, fmt.Sprintf("0.%d.0", i))
		os.Mkdir(path, 0755)
		os.WriteFile(filepath.Join(path, "codex"), nil, 0755)
		at := now.Add(-time.Duration(20-i) * 24 * time.Hour)
		os.Chtimes(path, at, at)
	}
	refs := References{Paths: map[string]bool{filepath.Join(group, "0.1.0", "codex"): true}, Revisions: map[string]bool{}, Versions: map[string]bool{}}
	read := func(context.Context) (References, error) { return refs, nil }
	cs, err := VersionCandidates(t.Context(), []string{group}, VersionLock(root), read, Default(), now, false)
	if err != nil || len(cs) != 3 {
		t.Fatal(cs, err)
	}
	// A new manifest published after collection still protects its executable.
	refs.Paths[filepath.Join(group, "0.2.0", "codex")] = true
	for _, c := range cs {
		if _, err := c.Remove(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []string{"0.1.0", "0.2.0", "0.5.0", "0.6.0"} {
		if _, err = os.Stat(filepath.Join(group, v)); err != nil {
			t.Fatal("protected version removed", v, err)
		}
	}
	for _, v := range []string{"0.3.0", "0.4.0"} {
		if _, err = os.Stat(filepath.Join(group, v)); !os.IsNotExist(err) {
			t.Fatal("old version retained", v, err)
		}
	}
	lease, err := VersionLease(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer lease()
	x := flock.New(VersionLock(root))
	defer x.Close()
	if ok, err := x.TryLock(); err != nil || ok {
		t.Fatal("publication lease ignored", ok, err)
	}
}

func TestReferenceCensusMapsProjectPathsAndRejectsMalformedMetadata(t *testing.T) {
	root := t.TempDir()
	tools := t.TempDir()
	session := filepath.Join(root, "sessions", "s")
	os.MkdirAll(session, 0700)
	os.WriteFile(filepath.Join(session, "session.json"), []byte(`{"id":"s","supervisor":"/cxz/state/data/runtime-binaries/abc/cxz","agent":"/cxz/tools/codex/0.1.0/bin/codex"}`), 0600)
	os.WriteFile(filepath.Join(root, "cxz-update.json"), []byte(`{"release":{"revision":"current","ancestors":["old"]}}`), 0600)
	r, err := ReadReferences(t.Context(), []string{root}, tools)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Protects(filepath.Join(root, "runtime-binaries", "abc")) || !r.Protects(filepath.Join(tools, "codex", "0.1.0")) || r.Revisions["old"] {
		t.Fatal(r)
	}
	os.WriteFile(filepath.Join(session, "agent-update.json"), []byte("invalid"), 0600)
	if _, err = ReadReferences(t.Context(), []string{root}, tools); err == nil {
		t.Fatal("unknown rollback references ignored")
	}
}

func TestBusyRuntimeWithoutIdentityPreventsExecutableCleanup(t *testing.T) {
	root := t.TempDir()
	daemon := flock.New(filepath.Join(root, "daemon.lock"))
	if err := daemon.Lock(); err != nil {
		t.Fatal(err)
	}
	defer daemon.Close()
	if _, err := ReadReferences(t.Context(), []string{root}, t.TempDir()); err == nil {
		t.Fatal("old live runtime incorrectly treated as unreferenced")
	}
}
