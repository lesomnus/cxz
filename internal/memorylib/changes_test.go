package memorylib

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func TestChangesHostEditsDeletionAndIndependentReaders(t *testing.T) {
	root, a, b := fixture(t)
	initial := must(t, a, Request{Action: "changes"})
	if initial.Cursor == "" || !initial.Baseline || len(initial.Changes) != 0 {
		t.Fatal(initial)
	}
	saved := must(t, a, Request{Action: "update", Document: "handoff.md", Content: "first"})
	delta := must(t, b, Request{Action: "changes", Cursor: initial.Cursor})
	if len(delta.Changes) != 2 {
		t.Fatal(delta)
	}
	for _, c := range delta.Changes {
		if c.Kind != "created" || c.MemoryID != a.OwnID() {
			t.Fatal(c)
		}
	}
	restarted := New(root, a.Session)
	same := must(t, restarted, Request{Action: "changes", Cursor: initial.Cursor})
	if !reflect.DeepEqual(delta, same) {
		t.Fatal("readers do not have independent persistent cursors", delta, same)
	}
	must(t, a, Request{Action: "update", Document: "handoff.md", Content: "first", Revision: saved.Revision})
	unchanged := must(t, a, Request{Action: "changes", Cursor: delta.Cursor})
	if len(unchanged.Changes) != 0 || unchanged.Cursor != delta.Cursor {
		t.Fatal(unchanged)
	}
	path := filepath.Join(a.root, a.OwnID(), "handoff.md")
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("host edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
	edited := must(t, a, Request{Action: "changes", Cursor: delta.Cursor})
	if len(edited.Changes) != 1 || edited.Changes[0].Kind != "updated" || edited.Changes[0].Revision != revision([]byte("host edit")) {
		t.Fatal(edited)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	deleted := must(t, a, Request{Action: "changes", Cursor: edited.Cursor})
	if len(deleted.Changes) != 1 || deleted.Changes[0].Kind != "deleted" || deleted.Changes[0].Document != "handoff.md" {
		t.Fatal(deleted)
	}
	must(t, a, Request{Action: "rename", Name: "Renamed"})
	renamed := must(t, a, Request{Action: "changes", Cursor: deleted.Cursor})
	if len(renamed.Changes) != 1 || renamed.Changes[0].Name != "Renamed" || renamed.Changes[0].Kind != "updated" {
		t.Fatal(renamed)
	}
	must(t, a, Request{Action: "delete"})
	removed := must(t, a, Request{Action: "changes", Cursor: renamed.Cursor})
	if len(removed.Changes) != 1 || removed.Changes[0].Kind != "deleted" || removed.Changes[0].Document != "" {
		t.Fatal(removed)
	}
	must(t, a, Request{Action: "restore", ID: a.OwnID()})
	restored := must(t, a, Request{Action: "changes", Cursor: removed.Cursor})
	if len(restored.Changes) != 1 || restored.Changes[0].Kind != "created" {
		t.Fatal(restored)
	}
}

func TestChangesPaginationWithConcurrentMutations(t *testing.T) {
	_, a, _ := fixture(t)
	saved := must(t, a, Request{Action: "update", Document: "a.md", Content: "old"})
	must(t, a, Request{Action: "update", Document: "z.md", Content: "delete me"})
	first := must(t, a, Request{Action: "changes", Limit: 1})
	if !first.HasMore || len(first.Changes) != 1 || first.Changes[0].Document != "" {
		t.Fatal(first)
	}
	second := must(t, a, Request{Action: "changes", Cursor: first.Cursor, Limit: 1})
	if !second.HasMore || second.Changes[0].Document != "a.md" {
		t.Fatal(second)
	}
	// Change an already delivered entry, delete an undelivered entry, and add one
	// sorting before the current baseline position. None may be lost.
	must(t, a, Request{Action: "update", Document: "a.md", Content: "new", Revision: saved.Revision})
	must(t, a, Request{Action: "update", Document: "0.md", Content: "new"})
	if err := os.Remove(filepath.Join(a.root, a.OwnID(), "z.md")); err != nil {
		t.Fatal(err)
	}
	cursor := second.Cursor
	got := map[string]Change{}
	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("pagination did not finish")
		}
		page := must(t, a, Request{Action: "changes", Cursor: cursor, Limit: 1})
		for _, c := range page.Changes {
			got[c.Document] = c
		}
		cursor = page.Cursor
		if !page.HasMore {
			break
		}
	}
	if len(got) != 3 || got["a.md"].Kind != "updated" || got["0.md"].Kind != "created" || got["z.md"].Kind != "deleted" {
		t.Fatal(got)
	}
	if page := must(t, a, Request{Action: "changes", Cursor: cursor}); len(page.Changes) != 0 {
		t.Fatal(page)
	}
}

func TestChangesCursorValidationAndReset(t *testing.T) {
	root, a, _ := fixture(t)
	initial := must(t, a, Request{Action: "changes"})
	other := New(root, core.Session{ID: core.ID(), ProjectID: "other"})
	if _, err := other.Do(context.Background(), Request{Action: "changes", Cursor: initial.Cursor}); err == nil {
		t.Fatal("accepted another project's cursor")
	}
	for _, q := range []Request{{Action: "changes", Cursor: "invalid"}, {Action: "changes", Limit: -1}, {Action: "changes", Limit: 501}} {
		if _, err := a.Do(context.Background(), q); err == nil {
			t.Fatal("accepted invalid request", q)
		}
	}
	c, err := decodeCursor(initial.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	c.Sequence++
	if _, err := a.Do(context.Background(), Request{Action: "changes", Cursor: encodeCursor(c)}); err == nil {
		t.Fatal("accepted future cursor")
	}
	if err := os.Remove(filepath.Join(a.root, changeIndexFile)); err != nil {
		t.Fatal(err)
	}
	reset := must(t, a, Request{Action: "changes", Cursor: initial.Cursor})
	if !reset.ResetRequired || reset.Cursor != "" {
		t.Fatal(reset)
	}
	fresh := must(t, a, Request{Action: "changes"})
	if fresh.ResetRequired || fresh.Cursor == "" {
		t.Fatal(fresh)
	}
}

func TestChangesRetentionAndFailedScan(t *testing.T) {
	_, a, _ := fixture(t)
	must(t, a, Request{Action: "update", Document: "a.md", Content: "a"})
	initial := must(t, a, Request{Action: "changes"})
	// Simulate retention overflow without thousands of disk scans.
	path := filepath.Join(a.root, changeIndexFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var idx changeIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatal(err)
	}
	idx.Sequence++
	idx.Items["deleted"] = Change{Sequence: idx.Sequence, Kind: "deleted"}
	idx.prune(0)
	raw, _ = json.Marshal(idx)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	expired := must(t, a, Request{Action: "changes", Cursor: initial.Cursor})
	if !expired.ResetRequired {
		t.Fatal(expired)
	}
	baseline := must(t, a, Request{Action: "changes", Limit: 1})
	if baseline.ResetRequired || !baseline.HasMore {
		t.Fatal(baseline)
	}
	// Old live entries remain available even when older than the tombstone floor.
	next := must(t, a, Request{Action: "changes", Cursor: baseline.Cursor, Limit: 1})
	if next.ResetRequired || len(next.Changes) != 1 {
		t.Fatal(next)
	}
	if err := os.WriteFile(filepath.Join(a.root, a.OwnID(), "a.md"), []byte{0}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Do(context.Background(), Request{Action: "changes", Cursor: next.Cursor}); err == nil {
		t.Fatal("invalid host edit accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(raw) {
		t.Fatal("failed scan advanced index")
	}
}

func TestChangesSnapshotsAndDocumentRestore(t *testing.T) {
	_, a, _ := fixture(t)
	saved := must(t, a, Request{Action: "update", Document: "기억.md", Content: "decision"})
	base := must(t, a, Request{Action: "changes"})
	snapshot := must(t, a, Request{Action: "snapshot", Name: "Saved"})
	delta := must(t, a, Request{Action: "changes", Cursor: base.Cursor})
	if len(delta.Changes) != 2 {
		t.Fatal(delta)
	}
	for _, c := range delta.Changes {
		if c.MemoryID != snapshot.Memory.ID || c.Kind != "created" {
			t.Fatal(c)
		}
	}
	must(t, a, Request{Action: "delete", Document: "기억.md", Revision: saved.Revision})
	deleted := must(t, a, Request{Action: "changes", Cursor: delta.Cursor})
	if len(deleted.Changes) != 1 || deleted.Changes[0].Kind != "deleted" {
		t.Fatal(deleted)
	}
	trash := must(t, a, Request{Action: "deleted_documents"})
	must(t, a, Request{Action: "restore", Document: trash.Documents[0].Name})
	restored := must(t, a, Request{Action: "changes", Cursor: deleted.Cursor})
	if len(restored.Changes) != 1 || restored.Changes[0].Kind != "created" || restored.Changes[0].Revision != saved.Revision {
		t.Fatal(restored)
	}
	must(t, a, Request{Action: "delete", ID: snapshot.Memory.ID})
	removed := must(t, a, Request{Action: "changes", Cursor: restored.Cursor})
	if len(removed.Changes) != 2 {
		t.Fatal(removed)
	}
	must(t, a, Request{Action: "purge", ID: snapshot.Memory.ID, Trash: true})
	purged := must(t, a, Request{Action: "changes", Cursor: removed.Cursor})
	if len(purged.Changes) != 0 {
		t.Fatal("purging trash should not delete live items again", purged)
	}
}

func TestChangesConcurrentReaders(t *testing.T) {
	_, a, b := fixture(t)
	baseline := must(t, a, Request{Action: "changes"})
	must(t, a, Request{Action: "update", Document: "a.md", Content: "decision"})
	var wg sync.WaitGroup
	replies := make(chan Reply, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			reply, err := b.Do(context.Background(), Request{Action: "changes", Cursor: baseline.Cursor})
			if err != nil {
				errs <- err
			} else {
				replies <- reply
			}
		})
	}
	wg.Wait()
	close(replies)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var previous *Reply
	for reply := range replies {
		if len(reply.Changes) != 2 {
			t.Fatal(reply)
		}
		if previous != nil && !reflect.DeepEqual(*previous, reply) {
			t.Fatal("concurrent readers received inconsistent deltas")
		}
		previous = &reply
	}
}
