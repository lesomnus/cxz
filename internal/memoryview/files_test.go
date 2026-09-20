package memoryview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
)

func fixture(t *testing.T, agent string) (string, Query, string) {
	t.Helper()
	root := t.TempDir()
	q := Query{Session: core.Session{CreateID: "stable-create", Kind: agent, Account: "default"}}
	config := accounts.Config(accounts.SessionRoot(root, q.Session.CreateID), "default")
	if err := os.MkdirAll(config, 0700); err != nil {
		t.Fatal(err)
	}
	return root, q, config
}
func put(t *testing.T, root, name, body string) {
	t.Helper()
	target := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestRetainedDataWithoutAgent(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		t.Run(agent, func(t *testing.T) {
			root, q, config := fixture(t, agent)
			name := "projects/old-project/memory/MEMORY.md"
			if agent == "codex" {
				name = "memories/notes.md"
			}
			put(t, config, name, "remember this")
			put(t, config, "auth.json", "secret")
			put(t, config, ".credentials.json", "secret")
			page, err := Read(t.Context(), root, q)
			if err != nil || !page.Directory || len(page.Entries) != 1 {
				t.Fatal(page, err)
			}
			q.Path = name
			page, err = Read(t.Context(), root, q)
			if err != nil || page.Content != "remember this" {
				t.Fatal(page, err)
			}
			q.Session.Workspace = "/renamed"
			q.Session.Title = "renamed"
			page, err = Read(t.Context(), root, q)
			if err != nil || page.Content != "remember this" {
				t.Fatal("rename changed identity", page, err)
			}
			q.Session.CreateID = "another-session"
			q.Path = ""
			page, err = Read(t.Context(), root, q)
			if err != nil || len(page.Entries) != 0 || page.Note == "" {
				t.Fatal("crossed session boundary", page, err)
			}
		})
	}
}
func TestMemoryPathIsolationAndLimits(t *testing.T) {
	root, q, config := fixture(t, "claude")
	put(t, config, "CLAUDE.md", strings.Repeat("a", FileLimit+100))
	put(t, config, "projects/fifo-placeholder", "")
	put(t, config, "projects/auth.json", "secret")
	if err := os.Symlink(filepath.Join(config, "CLAUDE.md"), filepath.Join(config, "projects", "link")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../auth.json", "/etc/passwd", "auth.json", "projects/../auth.json", "projects/auth.json", "projects/link"} {
		q.Path = name
		if _, err := Read(t.Context(), root, q); err == nil {
			t.Fatal("accepted", name)
		}
	}
	q.Path = "CLAUDE.md"
	page, err := Read(t.Context(), root, q)
	if err != nil || !page.Truncated || len(page.Content) != FileLimit {
		t.Fatal(page.Truncated, len(page.Content), err)
	}
	q.Path = "projects"
	page, err = Read(t.Context(), root, q)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Name != "fifo-placeholder" {
		t.Fatal(page, err)
	}
	put(t, config, "projects/binary", "a\x00b")
	q.Path = "projects/binary"
	if _, err := Read(t.Context(), root, q); err == nil {
		t.Fatal("binary accepted")
	}
}
