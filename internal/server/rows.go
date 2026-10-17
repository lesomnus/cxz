package server

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/transcripthistory"
	"google.golang.org/grpc"
)

// rowsPage bounds one read of the projection, and a full page means there is
// more behind it: the loop drains rather than waiting for the next write.
const rowsPage = 256

// rowsHeartbeat says the stream is alive when it has nothing to say. That was
// the hole in cxz#188: a conversation stopped growing while the client still
// read as live, because a stream that never ends and never throws is invisible
// to the reader waiting on it.
//
// An event-less row is the frame, because it cannot be mistaken for a row:
// Tail always decodes one, and both the resource layer and the web client
// already pass over a row without an event.
//
// A variable so a test can shorten it; nothing else writes it.
var rowsHeartbeat = transcripthistory.Heartbeat

// Rows streams the reading projection.
//
// Watch and this answer the same journal and are not alternatives: Watch
// carries records as they were written, which is what a derivation reads, and
// this carries the rows a person reads. A client that renders a conversation
// wants exactly one of them, and until now there was only the record stream --
// so every client reduced records into rows itself, in its own language. See
// docs/plans/reading-projection.md.
//
// The cursor is over row versions. Folding a turn's metrics onto its response
// publishes that response again at the sequence of the record that completed
// it, so the same seq arrives twice and the second one is the correction. A
// reader replaces by seq and resumes from updated.
func (s *Server) Rows(r *api.RowsRequest, stream grpc.ServerStreamingServer[api.Row]) error {
	if s.manager != nil {
		return s.rowsRemote(r, stream)
	}
	ctx := stream.Context()
	m, err := s.manifest(ctx, r.SessionId)
	if err != nil {
		return err
	}
	cursor := r.AfterUpdated
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	// Due immediately, so the first heartbeat goes out as soon as the stream has
	// caught up: a reader learns that silence will be reported to it within one
	// round trip rather than after the first idle interval.
	last := time.Now().Add(-rowsHeartbeat)
	for {
		batch, err := s.syncedRows(ctx, m, cursor)
		if err != nil {
			return err
		}
		for _, v := range batch {
			if err = stream.Send(v); err != nil {
				return err
			}
			cursor = v.Updated
			last = time.Now()
		}
		if len(batch) == rowsPage {
			continue
		}
		if time.Since(last) >= rowsHeartbeat {
			if err = stream.Send(&api.Row{Updated: cursor}); err != nil {
				return err
			}
			last = time.Now()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// syncedRows brings the index up to the journal and reads what is new, holding
// the session's projection lock for both so a concurrent page cannot see a
// half-built index. Transcript does the same two steps for one page.
func (s *Server) syncedRows(ctx context.Context, m core.Session, cursor uint64) ([]*api.Row, error) {
	p, err := s.lockProjection(ctx, m)
	if err != nil {
		return nil, err
	}
	defer p.mu.Unlock()
	if err = transcripthistory.Sync(ctx, s.db, m.ID, decodeTranscriptEvent); err != nil {
		return nil, err
	}
	return transcripthistory.Tail(ctx, s.db, m.ID, cursor, rowsPage, decodeTranscriptEvent)
}

func (s *Server) rowsRemote(r *api.RowsRequest, stream grpc.ServerStreamingServer[api.Row]) error {
	conn, client, err := s.manager.ClientFor(stream.Context(), r.SessionId)
	if err != nil {
		return err
	}
	defer conn.Close()
	remote, err := client.Rows(stream.Context(), r)
	if err != nil {
		return err
	}
	// Rows are not cached on the way through. The manager's cache is of records,
	// which is what it rebuilds a projection from; a row is derived from those
	// and keeping a second copy would be a second thing to invalidate.
	//
	// Heartbeats are forwarded rather than generated here, so what a reader
	// measures is the runtime's liveness and not this proxy's. A runtime too old
	// to send them leaves the reader where it was before cxz#188: silent, and
	// unable to tell. That is why the client arms its deadline only once it has
	// seen a heartbeat.
	for {
		v, err := remote.Recv()
		if err != nil {
			return err
		}
		if err := stream.Send(v); err != nil {
			return err
		}
	}
}
