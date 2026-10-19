package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/supervisor"
)

// The first look at a journal in a process is always a reset, so a session
// whose history has been compacted used to have its checkpoint rewritten on
// every restart. A rewrite is a new rowid, which is how the reading projection
// tells a record landing behind its cursor from an append -- so the first
// conversation opened after a restart was reprojected from the beginning, which
// on a real installation was 4s on a 103k-record session and 18s on a 146k one.
//
// The rowid high-water mark is the assertion because it is what the reading
// projection actually asks about. See cxz#227.
func TestARestartDoesNotRewriteACompactedSessionsCheckpoint(t *testing.T) {
	s, m, l := projectionFixture(t)
	ctx := context.Background()
	for i := 0; i < 8; i++ {
		kind, text := "assistant", "a"
		if i == 7 {
			kind, text = "state", "idle"
		}
		l.Append(core.Event{SessionID: m.ID, RunID: "run", Kind: kind, Text: text})
	}
	p, err := s.lockProjection(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	p.mu.Unlock()
	before := l.All()
	payload, _ := json.Marshal(core.HistoryCheckpoint{Version: 1, Snapshot: supervisor.Replay(before[:4])})
	if err = l.Compact(core.Event{SessionID: m.ID, Seq: 4, Kind: core.HistoryCheckpointKind, Payload: payload}); err != nil {
		t.Fatal(err)
	}

	written := func() uint64 {
		t.Helper()
		var v uint64
		if e := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(rowid),0) FROM events").Scan(&v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	// A restart is a server that has never looked at this journal.
	restart := func() {
		t.Helper()
		s.projectionMu.Lock()
		s.projections = nil
		s.projectionMu.Unlock()
		q, e := s.lockProjection(ctx, m)
		if e != nil {
			t.Fatal(e)
		}
		q.mu.Unlock()
	}
	restart()
	// The checkpoint has to reach the cache once: it stands where compaction
	// removed a record, and a reader that never sees it cannot tell that the
	// history below it is gone.
	var kind string
	if err = s.db.QueryRowContext(ctx, "SELECT json_extract(data,'$.kind') FROM events WHERE session_id=? AND seq=4", m.ID).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != core.HistoryCheckpointKind {
		t.Fatal("the checkpoint did not replace the record compaction removed, found", kind)
	}
	at := written()
	for i := 0; i < 3; i++ {
		restart()
	}
	if now := written(); now != at {
		t.Fatal("a restart wrote", now-at, "records it already had, which reprojects the session")
	}
	// A conversation still reads, and reads the rows the checkpoint left behind.
	page, err := s.Transcript(ctx, &api.TranscriptRequest{SessionId: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) == 0 {
		t.Fatal("the conversation is empty after a restart")
	}
	for _, e := range page.Events {
		if e.Seq < 4 {
			t.Fatal("a compacted record survived in the projection", e.Seq)
		}
	}
}
