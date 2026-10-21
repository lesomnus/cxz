package sessionstore

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
	"github.com/lesomnus/cxz/internal/transcripthistory"
	"github.com/lesomnus/payday/config"
	"google.golang.org/protobuf/proto"
)

// conversation is a journal with the shapes that make the reduction interesting:
// a tool call assembled from three records, an approval resolved after the fact,
// a turn that closes and folds its metrics onto an earlier response, kinds that
// produce no row at all, and a second run after the first ends.
func conversation(t *testing.T, dir string) []core.Event {
	t.Helper()
	l, err := journal.Open(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	write := func(e core.Event) {
		t.Helper()
		if _, err := l.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	run := "run-1"
	write(core.Event{SessionID: "s", RunID: run, Kind: "vendor", Text: "vendor-1"})
	write(core.Event{SessionID: "s", RunID: run, Kind: "input", Text: "build it"})
	write(core.Event{SessionID: "s", RunID: run, Kind: "assistant", Text: "on it", Response: &core.ResponseMetadata{Phase: "final_answer"}})
	write(core.Event{SessionID: "s", RunID: run, Kind: "tool_call", RequestID: "t1", Text: "Bash", Payload: json.RawMessage(`{"input":{"command":"go build ./..."}}`)})
	write(core.Event{SessionID: "s", RunID: run, Kind: "approval", RequestID: "a1", Text: "Bash", Payload: json.RawMessage(`{"tool_use_id":"t1"}`)})
	write(core.Event{SessionID: "s", RunID: run, Kind: "approval_resolved", RequestID: "a1", Text: "allowed"})
	write(core.Event{SessionID: "s", RunID: run, Kind: "tool_output", RequestID: "t1", Payload: json.RawMessage(`{"chunk":"compiling"}`)})
	write(core.Event{SessionID: "s", RunID: run, Kind: "tool_result", RequestID: "t1", Payload: json.RawMessage(`{"is_error":false}`)})
	write(core.Event{SessionID: "s", RunID: run, Kind: "usage", Payload: json.RawMessage(`{"input_tokens":10}`)})
	write(core.Event{SessionID: "s", RunID: run, Kind: "state", Text: "idle"})
	write(core.Event{SessionID: "s", RunID: run, Kind: "turn_end", Text: "completed", Payload: json.RawMessage(`{"duration_ms":1200,"usage":{"input_tokens":10,"output_tokens":5}}`)})
	run = "run-2"
	write(core.Event{SessionID: "s", RunID: run, Kind: "input", Text: "again"})
	write(core.Event{SessionID: "s", RunID: run, Kind: "assistant", Text: "sure"})
	return l.All()
}

// today builds the same conversation the way the server does: the journal cached
// into an events table, then the reading projection over it.
func today(t *testing.T, dir string, events []core.Event) *api.TranscriptReply {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(dir, "today.db")
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: (&url.URL{Scheme: "file", Path: path}).String() + "?_pragma=journal_mode(WAL)", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.ExecContext(ctx, `CREATE TABLE events(session_id TEXT NOT NULL, seq INTEGER NOT NULL, data BLOB NOT NULL, PRIMARY KEY(session_id,seq))`); err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, `INSERT OR REPLACE INTO events VALUES(?,?,?)`, "s", e.Seq, b); err != nil {
			t.Fatal(err)
		}
	}
	if err = transcripthistory.Sync(ctx, db, "s", Decode); err != nil {
		t.Fatal(err)
	}
	page, err := transcripthistory.Page(ctx, db, &api.TranscriptRequest{SessionId: "s"}, Decode)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

// A row written by a session's own store and the same row written by the server
// today must be the same bytes. That is what lets the read switch later be a
// change of which file is opened rather than a change of what a person sees.
func TestAStoreAndTheServerReduceToTheSameRows(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	events := conversation(t, dir)
	want := today(t, dir, events)

	n, err := Convert(ctx, dir, "s")
	if err != nil {
		t.Fatal(err)
	}
	if n != len(events) {
		t.Fatal("converted", n, "of", len(events), "records")
	}
	s, err := Open(ctx, Path(dir), "s", false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := transcripthistory.Page(ctx, s.DB(), &api.TranscriptRequest{SessionId: "s"}, Decode)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) == 0 {
		t.Fatal("the store answered no rows")
	}
	if len(got.Events) != len(want.Events) {
		t.Fatalf("the store has %d rows, the server %d", len(got.Events), len(want.Events))
	}
	for i := range want.Events {
		if !proto.Equal(got.Events[i], want.Events[i]) {
			t.Fatalf("row %d differs\n store: %v\nserver: %v", i, got.Events[i], want.Events[i])
		}
	}
	if len(got.Metadata) != len(want.Metadata) {
		t.Fatalf("the store carries %d metadata records, the server %d", len(got.Metadata), len(want.Metadata))
	}
	for i := range want.Metadata {
		if !proto.Equal(got.Metadata[i], want.Metadata[i]) {
			t.Fatalf("metadata %d differs", i)
		}
	}
	if got.SnapshotSeq != want.SnapshotSeq || got.HasOlder != want.HasOlder || got.HasNewer != want.HasNewer {
		t.Fatal("the page fences differ", got.SnapshotSeq, want.SnapshotSeq)
	}
}

// Appending after a conversion costs the appended records, and the rows it
// produces are the ones a full conversion would have produced.
func TestAppendingAfterConversionMatchesConvertingTheWhole(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	events := conversation(t, dir)
	if _, err := Convert(ctx, dir, "s"); err != nil {
		t.Fatal(err)
	}
	// The journal stamps a time when a record has none, and the supervisor hands
	// the store what the journal stamped. Carry one here so that appending and
	// converting are compared on the records, not on when the test ran.
	later := []core.Event{
		{SessionID: "s", RunID: "run-2", Seq: events[len(events)-1].Seq + 1, TimeMS: 1_700_000_000_000, Kind: "tool_call", RequestID: "t2", Text: "Read", Payload: json.RawMessage(`{"input":{"file_path":"/tmp/x"}}`)},
		{SessionID: "s", RunID: "run-2", Seq: events[len(events)-1].Seq + 2, TimeMS: 1_700_000_000_001, Kind: "tool_result", RequestID: "t2", Payload: json.RawMessage(`{"is_error":true}`)},
	}
	s, err := Open(ctx, Path(dir), "s", true)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Append(ctx, later); err != nil {
		t.Fatal(err)
	}
	got, err := transcripthistory.Page(ctx, s.DB(), &api.TranscriptRequest{SessionId: "s"}, Decode)
	s.Close()
	if err != nil {
		t.Fatal(err)
	}
	// The same records, converted in one go into a second store.
	whole := t.TempDir()
	l, err := journal.Open(filepath.Join(whole, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	all := append(append([]core.Event{}, events...), later...)
	for _, e := range all {
		e.Seq = 0
		if _, err = l.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	l.Close()
	if _, err = Convert(ctx, whole, "s"); err != nil {
		t.Fatal(err)
	}
	ref, err := Open(ctx, Path(whole), "s", false)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Close()
	want, err := transcripthistory.Page(ctx, ref.DB(), &api.TranscriptRequest{SessionId: "s"}, Decode)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != len(want.Events) {
		t.Fatalf("appended gives %d rows, converting the whole gives %d", len(got.Events), len(want.Events))
	}
	for i := range want.Events {
		if !proto.Equal(got.Events[i], want.Events[i]) {
			t.Fatalf("row %d differs after appending\n appended: %v\n whole: %v", i, got.Events[i], want.Events[i])
		}
	}
}

// A conversion happens once and is finished rather than repeated: reading a
// journal is reading all of it, and after cxz#278 nothing appends to one. A
// conversion that was interrupted left no mark, so the next one finishes it.
func TestConvertHappensOnce(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	events := conversation(t, dir)
	n, err := Convert(ctx, dir, "s")
	if err != nil {
		t.Fatal(err)
	}
	if n != len(events) {
		t.Fatal("the first conversion took", n, "of", len(events))
	}
	for i := 0; i < 2; i++ {
		n, err := Convert(ctx, dir, "s")
		if err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatal("pass", i, "read the journal again:", n)
		}
	}
	s, err := Open(ctx, Path(dir), "s", false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var rows, records int
	if err = s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM transcript_rows`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err = s.DB().QueryRowContext(ctx, `SELECT COUNT(DISTINCT seq) FROM transcript_records`).Scan(&records); err != nil {
		t.Fatal(err)
	}
	if records != len(events) {
		t.Fatal("three conversions left", records, "records for", len(events), "events")
	}
	if rows == 0 {
		t.Fatal("no rows")
	}
}

// A store written by a newer cxz is not written by an older one. The number that
// decides it is `compat`, not the schema, so that adding to the shape does not
// lock out the build a rollback puts back.
func TestAStoreFromTheFutureRefusesAWriter(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, Path(dir), "s", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB().ExecContext(ctx, `INSERT OR REPLACE INTO meta VALUES('compat',?)`, Schema+1); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err = Open(ctx, Path(dir), "s", true); err == nil {
		t.Fatal("a store that demands a newer writer accepted this one")
	}
	// Reading is still allowed: a conversation recorded by a newer cxz is newer,
	// not invisible.
	r, err := Open(ctx, Path(dir), "s", false)
	if err != nil {
		t.Fatal("a newer store refused a reader:", err)
	}
	r.Close()
}

// A schema higher than this build's, with compat left where it was, is the
// additive case: the older build keeps writing.
func TestAnAdditiveSchemaStillAcceptsAnOlderWriter(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, Path(dir), "s", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB().ExecContext(ctx, `INSERT OR REPLACE INTO meta VALUES('schema',?)`, Schema+1); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(ctx, Path(dir), "s", true)
	if err != nil {
		t.Fatal("an additive change locked out this writer:", err)
	}
	defer s.Close()
	if err = s.Append(ctx, []core.Event{{SessionID: "s", Seq: 1, Kind: "input", Text: "hello"}}); err != nil {
		t.Fatal(err)
	}
}

// The store is stamped with what built it, so a reader can tell these rows from
// rows built by other rules.
func TestAStoreIsStamped(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, Path(dir), "s", true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	schema, err := s.Schema(ctx)
	if err != nil {
		t.Fatal(err)
	}
	compat, err := s.Compat(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var format int
	if err = s.DB().QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM meta WHERE key='rows_format'`).Scan(&format); err != nil {
		t.Fatal(err)
	}
	if schema != Schema || compat != Compat || format != transcripthistory.Format() {
		t.Fatal("stamp", schema, compat, format)
	}
	if _, err = os.Stat(Path(dir)); err != nil {
		t.Fatal(err)
	}
}
