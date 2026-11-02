package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type migratingRuntime struct {
	*taskFixture
	root string
}

func (f migratingRuntime) MemoryRoot() (string, bool) { return f.root, true }
func TestMemoryMigrationPreservesDocumentsTrashAndConcurrentEdits(t *testing.T) {
	f, _, db := taskStack(t)
	root := t.TempDir()
	ctx := t.Context()
	// Project-runtime identity makes retained memories of purged owners resolvable.
	raw, _ := json.Marshal(map[string]string{"ProjectID": f.p.Id})
	if err := os.WriteFile(filepath.Join(filepath.Dir(root), "runtime.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(filepath.Join(filepath.Dir(root), "runtime.json")) })
	hash := sha256.Sum256([]byte("old-project-id"))
	base := filepath.Join(root, "memories", hex.EncodeToString(hash[:12]))
	collection := "session-0123456789abcdef01234567"
	saved := "saved-abcdef0123456789abcdef01"
	write := func(group, name, content string, trash, snapshot bool) {
		t.Helper()
		path := filepath.Join(base, group)
		if trash {
			path = filepath.Join(base, "trash", group)
		}
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		manifest, _ := json.Marshal(map[string]any{"format": 1, "name": "default", "session": f.sessions[0].Id, "agent": "codex", "created": time.Now(), "snapshot": snapshot})
		if err := os.WriteFile(filepath.Join(path, "manifest.json"), manifest, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(collection, "notes.md", "한글과 Unicode ✓", false, false)
	write(collection, ".trash-0123456789abcdef01234567-old.md", "deleted document", false, false)
	write(saved, "notes.md", "saved original", false, true)
	write("session-abcdef0123456789abcdef01", "notes.md", "deleted collection", true, false)
	stack, err := Build(ctx, db, migratingRuntime{f, root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	layer, _ := resource.Find[Layer](stack)
	if err = layer.MigrateMemories(ctx); err != nil {
		t.Fatal(err)
	}
	if err = layer.MigrateMemories(ctx); err != nil {
		t.Fatal(err)
	}
	active, err := stack.Memory().List(ctx, resource.MemoryListRequest_builder{Project: projectRef(f.p.Id)}.Build())
	if err != nil || len(active.GetItems()) != 2 {
		t.Fatal(active, err)
	}
	trash, err := stack.Memory().List(ctx, resource.MemoryListRequest_builder{Project: projectRef(f.p.Id), Scope: resource.MemoryScope_MEMORY_SCOPE_TRASH}.Build())
	if err != nil || len(trash.GetItems()) != 2 {
		t.Fatal(trash, err)
	}
	var editable *resource.Memory
	for _, m := range active.GetItems() {
		if m.GetLabels()["legacy.snapshot"] != "true" {
			editable = m
		}
	}
	body, err := stack.Memory().Read(ctx, resource.MemoryReadRequest_builder{Ref: editable.Ref()}.Build())
	if err != nil || body.GetContent() != "한글과 Unicode ✓" {
		t.Fatal(body, err)
	}
	edited, err := stack.Memory().Patch(ctx, resource.MemoryPatchRequest_builder{Ref: editable.Ref(), Content: ptr("new API edit"), BaseRevision: ptr(uint64(1)), DateUpdated: body.GetMemory().GetDateUpdated()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	write(collection, "notes.md", "late legacy writer", false, false)
	if err = layer.MigrateMemories(ctx); err != nil {
		t.Fatal(err)
	}
	if err = layer.MigrateMemories(ctx); err != nil {
		t.Fatal(err)
	}
	active, err = stack.Memory().List(ctx, resource.MemoryListRequest_builder{Project: projectRef(f.p.Id)}.Build())
	if err != nil || len(active.GetItems()) != 3 {
		t.Fatal(active, err)
	}
	current, err := stack.Memory().Read(ctx, resource.MemoryReadRequest_builder{Ref: edited.Ref()}.Build())
	if err != nil || current.GetContent() != "new API edit" {
		t.Fatal(current, err)
	}
	original, err := os.ReadFile(filepath.Join(base, collection, "notes.md"))
	if err != nil || string(original) != "late legacy writer" {
		t.Fatal("source was mutated", err)
	}
	// The migration marker and first body version are in the same transaction.
	var versions int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_version WHERE memory_id=?`, projectid.String(edited.GetId())).Scan(&versions); err != nil || versions != 2 {
		t.Fatal(versions, err)
	}
}
