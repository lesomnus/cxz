package conversation

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionstore"
)

func (f *fixture) openDatabase(t *testing.T) *sessionstore.Store {
	t.Helper()
	db, err := sessionstore.Open(t.Context(), sessionstore.Path(core.Dir(f.store.Root, f.target.RuntimeID)), f.target.RuntimeID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLiteTailKeepsTheOriginalSnapshotAcrossPages(t *testing.T) {
	f := setup(t)
	f.add(t, "input", "journal-only event", nil)
	db := f.openDatabase(t)
	if err := db.Append(t.Context(), []core.Event{
		{Seq: 1, Kind: "input", Text: "archived request"},
		{Seq: 2, Kind: "input", Text: "match oldest"},
		{Seq: 3, Kind: "assistant", Text: "match earlier"},
		{Seq: 4, Kind: "tool_result", Text: "match tool"},
		{Seq: 5, Kind: "input", Text: "other topic"},
		{Seq: 6, Kind: "assistant", Text: "MATCH newest"},
		{Seq: 7, Kind: "raw", Raw: []byte(`{"message":"match raw"}`)},
	}); err != nil {
		t.Fatal(err)
	}

	stale, err := f.store.Search(t.Context(), Query{Session: "seal", Query: "journal-only"})
	if err != nil || len(stale.Hits) != 0 {
		t.Fatal("read a stale journal beside SQLite", stale, err)
	}
	page, err := f.store.Read(t.Context(), Query{
		Session: "seal", Query: "match", IgnoreCase: true, Tail: 2, Limit: 1,
	})
	if err != nil || page.SnapshotSeq != 7 || !page.HasMore || len(page.Bodies) != 1 || page.Bodies[0].Seq != 3 {
		t.Fatal("wrong first tail page", page, err)
	}
	if err := db.Append(t.Context(), []core.Event{{Seq: 8, Kind: "assistant", Text: "match after snapshot"}}); err != nil {
		t.Fatal(err)
	}
	continued, err := f.store.Read(t.Context(), Query{Cursor: page.NextCursor})
	if err != nil || continued.SnapshotSeq != 7 || continued.HasMore || len(continued.Bodies) != 1 || continued.Bodies[0].Seq != 6 {
		t.Fatal("tail changed during pagination", continued, err)
	}
	latest, err := f.store.Read(t.Context(), Query{Session: "seal", Tail: 1})
	if err != nil || len(latest.Bodies) != 1 || latest.Bodies[0].Seq != 8 {
		t.Fatal("new query missed the latest commit", latest, err)
	}
	raw, err := f.store.Read(t.Context(), Query{Session: "seal", View: "raw", Tail: 1})
	if err != nil || len(raw.Bodies) != 1 || raw.Bodies[0].Seq != 7 {
		t.Fatal("raw tail missed stored vendor output", raw, err)
	}
}

func TestSQLiteReadReportsSkippedRecordsAndRetainedBounds(t *testing.T) {
	f := setup(t)
	db := f.openDatabase(t)
	f.store.recordLimit = 4 << 10
	if err := db.Append(t.Context(), []core.Event{
		{Seq: 1, Kind: "input", Text: "trimmed"},
		{Seq: 2, Kind: "assistant", Text: "remaining"},
		{Seq: 3, Kind: "assistant", Text: strings.Repeat("x", 8<<10)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Trim(t.Context(), core.Event{
		SessionID: f.target.RuntimeID, Seq: 1, Kind: core.HistoryCheckpointKind,
		Payload: []byte(`{"version":1,"snapshot":{"last_seq":1}}`),
	}); err != nil {
		t.Fatal(err)
	}
	page, err := f.store.Read(t.Context(), Query{Session: "seal", Tail: 2})
	if err != nil || !slices.Equal(page.UnreadableSeqs, []uint64{3}) || !page.HistoryTruncated ||
		page.TrimmedThrough != 1 || page.FirstSeq != 2 || page.SnapshotSeq != 3 ||
		len(page.Bodies) != 1 || page.Bodies[0].Seq != 2 {
		t.Fatal("stored retention or record limits were lost", page, err)
	}
}

func TestConversationStoreRejectsSymlinks(t *testing.T) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		t.Run(sessionstore.Name+suffix, func(t *testing.T) {
			f := setup(t)
			db := f.openDatabase(t)
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			path := sessionstore.Path(core.Dir(f.store.Root, f.target.RuntimeID)) + suffix
			target := filepath.Join(t.TempDir(), filepath.Base(path))
			if suffix == "" {
				if err := os.Rename(path, target); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(target, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := f.store.Read(t.Context(), Query{Session: "seal"}); err == nil || !strings.Contains(err.Error(), "invalid conversation store file") {
				t.Fatal("read followed a store symlink", err)
			}
		})
	}
}
