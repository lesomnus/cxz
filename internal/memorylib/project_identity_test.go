package memorylib

import (
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/projectid"
	"os"
	"path/filepath"
	"testing"
)

func TestUUIDProjectKeepsExistingMemoryDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	legacy := "0123456789abcdef01234567"
	session := core.Session{ID: core.ID(), ProjectID: legacy}
	old := New(root, session)
	must(t, old, Request{Action: "update", Document: "notes.md", Content: "Retained memory"})
	if err := os.WriteFile(filepath.Join(filepath.Dir(root), "runtime.json"), []byte(`{"ProjectID":"`+legacy+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	session.ProjectID = projectid.Canonical(legacy)
	current := New(root, session)
	if current.root != old.root {
		t.Fatal("project memory was relocated", current.root, old.root)
	}
	if got := must(t, current, Request{Action: "read", Document: "notes.md"}); got.Content != "Retained memory" {
		t.Fatal(got)
	}
	session.ProjectID = projectid.New()
	if New(root, session).root == old.root {
		t.Fatal("runtime mapping crossed project scope")
	}
}
