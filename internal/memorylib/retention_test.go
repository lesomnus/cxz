package memorylib

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrashGraceDeletionTimeAndLiveMemoryProtection(t *testing.T) {
	root, a, _ := fixture(t)
	v := must(t, a, Request{Action: "update", Document: "old.md", Content: "saved"})
	old := time.Now().Add(-100 * 24 * time.Hour)
	path := filepath.Join(a.root, a.OwnID(), "old.md")
	os.Chtimes(path, old, old)
	must(t, a, Request{Action: "delete", Document: "old.md", Revision: v.Revision})
	must(t, a, Request{Action: "update", Document: "live.md", Content: "keep"})
	if n, err := SweepTrash(t.Context(), root, time.Now(), 30); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	barrier := filepath.Join(a.root, ".retention-start")
	os.Chtimes(barrier, old, old)
	matches, _ := filepath.Glob(filepath.Join(a.root, a.OwnID(), ".trash-*"))
	if len(matches) != 1 {
		t.Fatal(matches)
	}
	// A document's original modification date must not make a fresh delete expire.
	if n, err := SweepTrash(t.Context(), root, time.Now(), 30); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	os.Chtimes(matches[0], old, old)
	if n, err := SweepTrash(t.Context(), root, time.Now(), 30); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if got := must(t, a, Request{Action: "read", Document: "live.md"}); got.Content != "keep" {
		t.Fatal(got)
	}
}
