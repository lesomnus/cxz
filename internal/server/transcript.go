package server

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/eventview"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/transcripthistory"
)

func decodeTranscriptEvent(b []byte) (*api.Event, error) {
	var e core.Event
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return eventview.FromCore(e), nil
}
func (s *Server) Transcript(ctx context.Context, r *api.TranscriptRequest) (*api.TranscriptReply, error) {
	if s.manager != nil {
		return s.manager.Transcript(ctx, r)
	}
	m, err := s.manifest(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	p, err := s.lockProjection(ctx, m)
	if err != nil {
		return nil, err
	}
	defer p.mu.Unlock()
	db := s.conversation(p)
	// A session that maintains its own rows is not synced from here: this
	// server reads that store, it does not write it.
	if !p.owned {
		if err = transcripthistory.SyncWith(ctx, db, r.SessionId, decodeTranscriptEvent, &p.transcript); err != nil {
			return nil, err
		}
	}
	return transcripthistory.Page(ctx, db, r, decodeTranscriptEvent)
}
func (s *Server) EventDetails(ctx context.Context, r *api.EventDetailsRequest) (*api.EventBatch, error) {
	if s.manager != nil {
		return s.manager.EventDetails(ctx, r)
	}
	m, err := s.manifest(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	p, err := s.lockProjection(ctx, m)
	if err != nil {
		return nil, err
	}
	defer p.mu.Unlock()
	db := s.conversation(p)
	if !p.owned {
		if err = transcripthistory.SyncWith(ctx, db, r.SessionId, decodeTranscriptEvent, &p.transcript); err != nil {
			return nil, err
		}
	}
	return transcripthistory.Details(ctx, db, r, decodeTranscriptEvent)
}
