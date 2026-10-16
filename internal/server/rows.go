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
		}
		if len(batch) == rowsPage {
			continue
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
