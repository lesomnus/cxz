package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"google.golang.org/grpc"
)

// rowSink collects the stream's frames and ends it when the test has what it
// asked for, which is how a reader leaving ends a real one.
type rowSink struct {
	grpc.ServerStream
	ctx    context.Context
	cancel context.CancelFunc
	frames []*api.Row
	done   func([]*api.Row) bool
}

func (s *rowSink) Context() context.Context { return s.ctx }
func (s *rowSink) Send(v *api.Row) error {
	s.frames = append(s.frames, v)
	if s.done(s.frames) {
		s.cancel()
	}
	return nil
}

func rowsUntil(t *testing.T, s *Server, r *api.RowsRequest, done func([]*api.Row) bool) []*api.Row {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sink := &rowSink{ctx: ctx, cancel: cancel, done: done}
	err := s.Rows(r, sink)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("the stream ended for another reason", err, len(sink.frames))
	}
	return sink.frames
}

// An open stream that has nothing to send is indistinguishable from one that
// has stalled, which is how a conversation stopped growing while the client
// still read as live (cxz#188). The heartbeat is what makes them different, so
// it must arrive without waiting for an idle interval first -- a reader that
// has not seen one does not arm its deadline at all.
func TestIdleRowStreamHeartbeatsItsCursorAtOnce(t *testing.T) {
	s, m, _ := projectionFixture(t)
	start := time.Now()
	frames := rowsUntil(t, s, &api.RowsRequest{SessionId: m.ID, AfterUpdated: 7}, func(f []*api.Row) bool {
		return len(f) == 1
	})
	if frames[0].Event != nil {
		t.Fatal("a heartbeat carrying an event would be read as a row", frames[0])
	}
	if frames[0].Updated != 7 {
		t.Fatal("a heartbeat must not move the reader's cursor", frames[0].Updated)
	}
	if waited := time.Since(start); waited >= rowsHeartbeat {
		t.Fatal("the first heartbeat waited for an idle interval", waited)
	}
}

// Rows first, then heartbeats at the cursor those rows left behind: a reader
// that reconnects from a heartbeat must not skip a row, and must not be told
// twice that the stream is alive before it is.
func TestRowHeartbeatFollowsTheRowsAtTheirCursor(t *testing.T) {
	s, m, l := projectionFixture(t)
	was := rowsHeartbeat
	rowsHeartbeat = 20 * time.Millisecond
	t.Cleanup(func() { rowsHeartbeat = was })
	if _, err := l.AppendBatch([]core.Event{
		{SessionID: m.ID, Kind: "input", Text: "go", RunID: "run"},
		{SessionID: m.ID, Kind: "assistant", Text: "done", RunID: "run"},
	}); err != nil {
		t.Fatal(err)
	}
	frames := rowsUntil(t, s, &api.RowsRequest{SessionId: m.ID}, func(f []*api.Row) bool {
		return len(f) >= 4
	})
	var rows, beats []*api.Row
	for _, f := range frames {
		if f.Event == nil {
			beats = append(beats, f)
			continue
		}
		if len(beats) > 0 {
			t.Fatal("a row arrived after the stream said it had caught up", f.Event.Seq)
		}
		rows = append(rows, f)
	}
	if len(rows) != 2 || rows[0].Event.Kind != "input" || rows[1].Event.Kind != "assistant" {
		t.Fatal("the rows themselves changed", rows)
	}
	if len(beats) < 2 {
		t.Fatal("heartbeats stopped after one, so silence would still be silent", len(beats))
	}
	for _, b := range beats {
		if b.Updated != rows[1].Updated {
			t.Fatal("a heartbeat would resume a reader away from the last row it read", b.Updated, rows[1].Updated)
		}
	}
}
