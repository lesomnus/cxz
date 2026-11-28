package sessionstore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func bulkEvents(n int) []core.Event {
	events := make([]core.Event, n)
	for i := range events {
		events[i] = core.Event{
			SessionID: "child",
			Seq:       uint64(i + 1),
			TimeMS:    int64(1000 + i),
			Kind:      "assistant",
			Text:      "a line of a conversation being filled in bulk",
		}
	}
	return events
}

// The relaxed sync lasts exactly as long as the load. What it wrote is on disk
// when BulkFill returns -- a reopened store reads it back -- and the next
// writer of the file is recording at the usual durability again. See cxz#396.
func TestBulkFillIsDurableAndRestoresDurability(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "child.db")
	store, err := Open(ctx, path, "child", true)
	if err != nil {
		t.Fatal(err)
	}
	events := bulkEvents(5000)
	n, err := store.BulkFill(ctx, events, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(events) {
		t.Fatalf("filled %d of %d", n, len(events))
	}
	var synchronous int
	if err = store.DB().QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatal(err)
	}
	// 2 is FULL, which is what Durability names. A load that left this at 0
	// would make everything recorded afterwards unsafe.
	if synchronous != 2 {
		t.Fatalf("the load left synchronous at %d", synchronous)
	}
	// Recording continues through the ordinary path.
	if err = store.Append(ctx, []core.Event{{SessionID: "child", Seq: 5001, Kind: "input", Text: "after the fill"}}); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := Open(ctx, path, "child", false)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := again.Events(ctx)
	again.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != len(events)+1 {
		t.Fatalf("read back %d of %d", len(stored), len(events)+1)
	}
	if stored[0].Seq != 1 || stored[len(stored)-1].Seq != 5001 {
		t.Fatalf("sequence lost: %d..%d", stored[0].Seq, stored[len(stored)-1].Seq)
	}
}

// through is honoured the same way Fill honours it, so a resumed load does not
// rewrite what is already there.
func TestBulkFillSkipsWhatIsAlreadyStored(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "child.db"), "child", true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	events := bulkEvents(100)
	n, err := store.BulkFill(ctx, events, 60)
	if err != nil {
		t.Fatal(err)
	}
	if n != 40 {
		t.Fatalf("filled %d, want 40", n)
	}
}
