package memoryview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
)

func copyFixture(t *testing.T) (string, string, CopyQuery, string) {
	t.Helper()
	source, q, config := fixture(t, "claude")
	q.Session.ID = strings.Repeat("a", 24)
	put(t, config, "projects/old/memory/MEMORY.md", "remember the old project")
	put(t, config, "projects/old/memory/topic.md", "topic")
	targetRoot := t.TempDir()
	target := core.Session{ID: strings.Repeat("b", 24), CreateID: "target-id", Kind: "claude", Account: "other"}
	targetConfig := accounts.Config(accounts.SessionRoot(targetRoot, target.CreateID), target.Account)
	if err := os.MkdirAll(targetConfig, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, targetRoot, filepath.Join("sessions", target.ID, "supervisor.lock"), "")
	put(t, targetRoot, filepath.Join("session-profiles", filepath.Base(accounts.SessionRoot(targetRoot, target.CreateID)), "accounts", target.Account, "login.lock"), "")
	return source, targetRoot, CopyQuery{Source: q.Session, Target: target, Path: "projects/old/memory", TargetPath: "projects/new/memory"}, targetConfig
}
func TestCopyMemoryAcrossAccountsWithoutOverwrite(t *testing.T) {
	source, target, q, config := copyFixture(t)
	put(t, config, ".credentials.json", "keep-target-login")
	message, err := Copy(t.Context(), source, target, q)
	if err != nil || !strings.Contains(message, "Copied 3 entries") {
		t.Fatal(message, err)
	}
	b, err := os.ReadFile(filepath.Join(config, "projects/new/memory/MEMORY.md"))
	if err != nil || string(b) != "remember the old project" {
		t.Fatal(string(b), err)
	}
	if _, err = Copy(t.Context(), source, target, q); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatal("overwrote destination", err)
	}
	b, err = os.ReadFile(filepath.Join(config, ".credentials.json"))
	if err != nil || string(b) != "keep-target-login" {
		t.Fatal("changed target login", err)
	}
	matches, _ := filepath.Glob(filepath.Join(config, "projects/new/.cxz-memory-*"))
	if len(matches) != 0 {
		t.Fatal("staging leaked", matches)
	}
}
func TestCopyRejectsActiveTargetTraversalAndSymlinks(t *testing.T) {
	source, target, q, config := copyFixture(t)
	lock, err := core.Lock(filepath.Join(core.Dir(target, q.Target.ID), "supervisor.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Copy(t.Context(), source, target, q); err == nil || !strings.Contains(err.Error(), "stop the target") {
		t.Fatal("copied into running agent", err)
	}
	lock.Close()
	for _, dest := range []string{"../outside", "auth.json", "projects/../auth.json", "projects/new/.credentials.json"} {
		bad := q
		bad.TargetPath = dest
		if _, err = Copy(t.Context(), source, target, bad); err == nil {
			t.Fatal("accepted", dest)
		}
	}
	if err = os.Symlink(t.TempDir(), filepath.Join(config, "projects")); err != nil {
		t.Fatal(err)
	}
	if _, err = Copy(t.Context(), source, target, q); err == nil {
		t.Fatal("followed destination symlink")
	}
}
