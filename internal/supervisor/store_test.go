package supervisor

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionstore"
)

// withStore gives a display supervisor the store a running one has, in the
// directory its journal already lives in.
func withStore(t *testing.T, provider string) (*Supervisor, string) {
	t.Helper()
	s, _ := displaySupervisor(t, provider)
	dir := filepath.Dir(journalPathOf(t, s))
	s.openStore(context.Background(), dir)
	if s.store == nil {
		t.Fatal("the supervisor has no store")
	}
	t.Cleanup(s.closeStore)
	return s, dir
}

func journalPathOf(t *testing.T, s *Supervisor) string {
	t.Helper()
	p := s.log.Path()
	if p == "" {
		t.Fatal("the journal has no path")
	}
	return p
}

// Every record the journal accepts reaches the store. The journal is still the
// truth, so the test is not that the store is correct on its own but that it
// holds what the journal holds -- sequence for sequence.
func TestEveryJournalRecordReachesTheStore(t *testing.T) {
	ctx := context.Background()
	s, dir := withStore(t, "claude")

	// A single append, and a batch, which are the two paths into the journal.
	s.event("input", "build it", "", nil, nil)
	s.consume([]byte(`{"type":"assistant","message":{"model":"claude-test","content":[{"type":"text","text":"on it"}]}}`))
	s.event("state", "idle", "", nil, nil)

	journalled := s.log.All()
	if len(journalled) < 3 {
		t.Fatal("the journal has", len(journalled), "records")
	}
	store, err := sessionstore.Open(ctx, sessionstore.Path(dir), "s", false)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rows, err := store.DB().QueryContext(ctx, `SELECT seq, data FROM events WHERE session_id='s' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	stored := map[uint64]core.Event{}
	for rows.Next() {
		var seq uint64
		var b []byte
		if err = rows.Scan(&seq, &b); err != nil {
			t.Fatal(err)
		}
		var e core.Event
		if err = json.Unmarshal(b, &e); err != nil {
			t.Fatal(err)
		}
		stored[seq] = e
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(stored) != len(journalled) {
		t.Fatalf("the journal holds %d records, the store %d", len(journalled), len(stored))
	}
	for _, want := range journalled {
		got, ok := stored[want.Seq]
		if !ok {
			t.Fatalf("record %d (%s) never reached the store", want.Seq, want.Kind)
		}
		if got.Kind != want.Kind || got.Text != want.Text || got.RunID != want.RunID || got.TimeMS != want.TimeMS {
			t.Fatalf("record %d differs\n store: %+v\njournal: %+v", want.Seq, got, want)
		}
	}
}

// A store that cannot be written is not a durability failure. The journal is,
// and it is the one that kills a run -- so a session whose store is broken
// keeps recording.
func TestABrokenStoreDoesNotStopTheRecording(t *testing.T) {
	s, _ := withStore(t, "claude")
	// Closing it is the bluntest way to make every write fail.
	_ = s.store.Close()
	s.event("input", "still recorded", "", nil, nil)
	s.event("state", "idle", "", nil, nil)
	found := false
	for _, e := range s.log.All() {
		if e.Kind == "input" && e.Text == "still recorded" {
			found = true
		}
	}
	if !found {
		t.Fatal("the journal lost a record because the store could not take it")
	}
}

// Converting is what a session that predates the store does at start, and it
// picks up where it left off rather than starting over.
func TestOpeningConvertsWhatTheStoreIsMissing(t *testing.T) {
	ctx := context.Background()
	s, _ := displaySupervisor(t, "claude")
	s.event("input", "one", "", nil, nil)
	s.event("assistant", "two", "", nil, nil)
	dir := filepath.Dir(journalPathOf(t, s))

	s.openStore(ctx, dir)
	if s.store == nil {
		t.Fatal("no store")
	}
	through, err := s.store.Through(ctx)
	if err != nil {
		t.Fatal(err)
	}
	last := s.log.All()[len(s.log.All())-1].Seq
	if through != last {
		t.Fatalf("the store converted through %d, the journal ends at %d", through, last)
	}
	// Records written while the store was open, then a restart: the second open
	// has only the suffix to do.
	s.event("input", "three", "", nil, nil)
	s.closeStore()
	s.openStore(ctx, dir)
	defer s.closeStore()
	through, err = s.store.Through(ctx)
	if err != nil {
		t.Fatal(err)
	}
	last = s.log.All()[len(s.log.All())-1].Seq
	if through != last {
		t.Fatalf("after a restart the store is through %d, the journal ends at %d", through, last)
	}
}
