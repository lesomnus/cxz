package memorylib

import (
	"context"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func fixture(t *testing.T) (string, *Store, *Store) {
	t.Helper()
	root := t.TempDir()
	a := New(root, core.Session{ID: core.ID(), ProjectID: "project", Kind: "claude", Account: "work", CreateID: "a"})
	b := New(root, core.Session{ID: core.ID(), ProjectID: "project", Kind: "codex", Account: "work", CreateID: "b"})
	return root, a, b
}
func must(t *testing.T, s *Store, q Request) Reply {
	t.Helper()
	v, e := s.Do(context.Background(), q)
	if e != nil {
		t.Fatal(q, e)
	}
	return v
}
func TestSharedMemorySnapshotsForkAndIsolation(t *testing.T) {
	root, a, b := fixture(t)
	must(t, a, Request{Action: "update", Document: "handoff.md", Content: "PR pending; do not call it merged"})
	read := must(t, b, Request{Action: "read", ID: a.OwnID(), Document: "handoff.md"})
	if !strings.Contains(read.Content, "pending") {
		t.Fatal(read)
	}
	if _, e := b.Do(context.Background(), Request{Action: "update", ID: a.OwnID(), Document: "handoff.md", Content: "wrong"}); e == nil {
		t.Fatal("other session modified")
	}
	other := New(root, core.Session{ID: core.ID(), ProjectID: "other"})
	if _, e := other.Do(context.Background(), Request{Action: "read", ID: a.OwnID(), Document: "handoff.md"}); e == nil {
		t.Fatal("cross project read")
	}
	snap := must(t, a, Request{Action: "snapshot", Name: "Before merge"}).Memory.ID
	must(t, a, Request{Action: "update", Document: "handoff.md", Content: "Merged", Revision: read.Revision})
	if got := must(t, b, Request{Action: "read", ID: snap, Document: "handoff.md"}).Content; got != read.Content {
		t.Fatal("snapshot changed", got)
	}
	must(t, b, Request{Action: "fork", ID: snap})
	if got := must(t, b, Request{Action: "read", Document: "handoff.md"}).Content; got != read.Content {
		t.Fatal(got)
	}
	if _, e := b.Do(context.Background(), Request{Action: "fork", ID: snap}); e == nil {
		t.Fatal("fork overwrote target")
	}
	merged := must(t, a, Request{Action: "merge", IDs: []string{a.OwnID(), snap}, Name: "Combined"}).Memory.ID
	docs := must(t, a, Request{Action: "read", ID: merged}).Documents
	if len(docs) != 2 {
		t.Fatal(docs)
	}
}
func TestExternalEditsConflictTrashRestoreAndPurge(t *testing.T) {
	_, a, _ := fixture(t)
	v := must(t, a, Request{Action: "update", Document: "notes.md", Content: "one"})
	path := filepath.Join(a.root, a.OwnID(), "notes.md")
	if e := os.WriteFile(path, []byte("manual edit"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := a.Do(context.Background(), Request{Action: "update", Document: "notes.md", Content: "stale", Revision: v.Revision}); e == nil {
		t.Fatal("external edit overwritten")
	}
	v = must(t, a, Request{Action: "read", Document: "notes.md"})
	if v.Content != "manual edit" {
		t.Fatal(v)
	}
	must(t, a, Request{Action: "delete", Document: "notes.md", Revision: v.Revision})
	ds := must(t, a, Request{Action: "deleted_documents"}).Documents
	if len(ds) != 1 {
		t.Fatal(ds)
	}
	must(t, a, Request{Action: "restore", Document: ds[0].Name})
	snap := must(t, a, Request{Action: "snapshot", Name: "Keep"}).Memory.ID
	must(t, a, Request{Action: "delete", ID: snap})
	if got := must(t, a, Request{Action: "list", Trash: true}); len(got.Memories) != 1 {
		t.Fatal(got)
	}
	must(t, a, Request{Action: "restore", ID: snap})
	must(t, a, Request{Action: "delete", ID: snap})
	must(t, a, Request{Action: "purge", ID: snap, Trash: true})
	if len(must(t, a, Request{Action: "list", Trash: true}).Memories) != 0 {
		t.Fatal("purge failed")
	}
}
func TestConcurrentCompareAndSwapAndTraversal(t *testing.T) {
	_, a, _ := fixture(t)
	v := must(t, a, Request{Action: "update", Document: "notes.md", Content: "original"})
	var wg sync.WaitGroup
	ch := make(chan error, 2)
	for _, value := range []string{"one", "two"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			_, e := a.Do(context.Background(), Request{Action: "update", Document: "notes.md", Content: value, Revision: v.Revision})
			ch <- e
		}(value)
	}
	wg.Wait()
	close(ch)
	success := 0
	for e := range ch {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal(success)
	}
	for _, q := range []Request{{Action: "read", ID: "../sessions"}, {Action: "update", Document: "../secret.md"}, {Action: "restore", Document: ".trash-../../../../../../../../x-ok.md"}} {
		if _, e := a.Do(context.Background(), q); e == nil {
			t.Fatal("unsafe path accepted", q)
		}
	}
	secret := filepath.Join(t.TempDir(), "secret.md")
	os.WriteFile(secret, []byte("secret"), 0600)
	if e := os.Symlink(secret, filepath.Join(a.root, a.OwnID(), "link.md")); e == nil {
		if _, e := a.Do(context.Background(), Request{Action: "read", Document: "link.md"}); e == nil {
			t.Fatal("symlink followed")
		}
	}
}
func TestImportNativeExcludesCredentialsAndHistory(t *testing.T) {
	root, a, _ := fixture(t)
	config := accounts.Config(accounts.SessionRoot(root, a.Session.CreateID), a.Session.Account)
	for name, content := range map[string]string{"CLAUDE.md": "rules", "projects/p/memory/MEMORY.md": "facts", "projects/p/session.jsonl": "transcript", "auth.json": "secret", "skills/tool/SKILL.md": "unrelated"} {
		p := filepath.Join(config, name)
		os.MkdirAll(filepath.Dir(p), 0700)
		os.WriteFile(p, []byte(content), 0600)
	}
	must(t, a, Request{Action: "import"})
	ds := must(t, a, Request{Action: "read"}).Documents
	if len(ds) != 2 {
		t.Fatal(ds)
	}
	must(t, a, Request{Action: "import"})
	if len(must(t, a, Request{Action: "read"}).Documents) != 2 {
		t.Fatal("duplicate import")
	}
}

func TestRestoreWorkingMemoryAfterAgentRecreatesIt(t *testing.T) {
	_, a, _ := fixture(t)
	must(t, a, Request{Action: "update", Document: "notes.md", Content: "old"})
	must(t, a, Request{Action: "delete"})
	must(t, a, Request{Action: "update", Document: "notes.md", Content: "new"})
	must(t, a, Request{Action: "restore"})
	memories := must(t, a, Request{Action: "list"}).Memories
	if len(memories) != 2 {
		t.Fatal(memories)
	}
	if got := must(t, a, Request{Action: "read", Document: "notes.md"}).Content; got != "new" {
		t.Fatal(got)
	}
}
