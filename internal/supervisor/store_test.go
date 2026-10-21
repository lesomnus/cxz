package supervisor

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
	"github.com/lesomnus/cxz/internal/sessionstore"
)

// storeDir is where the display supervisor's log keeps its file.
func storeDir(t *testing.T, s *Supervisor) string {
	t.Helper()
	var path string
	if err := s.store.DB().QueryRow(`PRAGMA database_list`).Scan(new(int), new(string), &path); err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(path)
}

// Every event a session records is in its store, in sequence. The store is the
// record now -- there is no journal to compare it against, which is why this
// compares what the session holds in memory with what is on disk.
// See cxz#278.
func TestEveryRecordedEventIsInTheStore(t *testing.T) {
	ctx := context.Background()
	s, _ := displaySupervisor(t, "claude")
	s.store = s.log.Store()

	// A single append, and a batch, which are the two paths into the store.
	s.event("input", "build it", "", nil, nil)
	s.consume([]byte(`{"type":"assistant","message":{"model":"claude-test","content":[{"type":"text","text":"on it"}]}}`))
	s.event("state", "idle", "", nil, nil)

	held := s.log.All()
	if len(held) < 3 {
		t.Fatal("the session holds", len(held), "records")
	}
	rows, err := s.store.DB().QueryContext(ctx, `SELECT seq, data FROM events WHERE session_id='session' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var stored []core.Event
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
		if e.Seq != seq {
			t.Fatal("a record disagrees with the sequence it is stored under", e.Seq, seq)
		}
		stored = append(stored, e)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(stored) != len(held) {
		t.Fatalf("the session holds %d records and the store has %d", len(held), len(stored))
	}
	for i := range held {
		if stored[i].Seq != held[i].Seq || stored[i].Kind != held[i].Kind || stored[i].Text != held[i].Text {
			t.Fatalf("record %d differs: %+v vs %+v", i, held[i], stored[i])
		}
	}
}

// A session that was recorded before stores existed converts its journal when
// it opens, once, and picks up where a previous conversion left off rather than
// starting over.
func TestOpeningConvertsAJournalOnce(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	l, err := journal.Open(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err = l.Append(core.Event{SessionID: "session", RunID: "run", Kind: "input", Text: "said"}); err != nil {
			t.Fatal(err)
		}
	}
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}

	log, err := sessionstore.OpenLog(ctx, dir, "session")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(log.All()); got != 6 {
		t.Fatal("the conversation came back with", got, "records")
	}
	if _, err = log.Append(core.Event{SessionID: "session", RunID: "run", Kind: "input", Text: "more"}); err != nil {
		t.Fatal(err)
	}
	if err = log.Close(); err != nil {
		t.Fatal(err)
	}

	// Opening again converts nothing and loses nothing: the journal is never
	// written after this, and the record it was is already here.
	again, err := sessionstore.OpenLog(ctx, dir, "session")
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if got := len(again.All()); got != 7 {
		t.Fatal("reopening gave", got, "records")
	}
}

// The state a session records says which record it was reduced from, and it is
// written after the records rather than before. It used to carry a fence
// against the journal; there is no journal to be behind. See cxz#278.
func TestRecordedStateNamesTheLastRecord(t *testing.T) {
	ctx := context.Background()
	s, _ := displaySupervisor(t, "claude")
	s.store = s.log.Store()
	s.event("input", "build it", "", nil, nil)
	s.recordState(ctx)

	snap, found, err := s.store.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("no state was recorded")
	}
	if snap.LastSeq != s.snap.LastSeq || snap.LastSeq == 0 {
		t.Fatal("the state does not name the last record", snap.LastSeq, s.snap.LastSeq)
	}
	through, err := s.store.Through(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if through != snap.LastSeq {
		t.Fatal("the state claims more than the store holds", snap.LastSeq, through)
	}
}
