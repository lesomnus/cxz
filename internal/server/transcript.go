package server

import (
	"context"
	"encoding/json"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/transcripthistory"
)

func decodeTranscriptEvent(b []byte) (*api.Event, error) {
	var e core.Event
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return pbEvent(e), nil
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
	if err = transcripthistory.Sync(ctx, s.db, r.SessionId, decodeTranscriptEvent); err != nil {
		return nil, err
	}
	return transcripthistory.Page(ctx, s.db, r, decodeTranscriptEvent)
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
	if err = transcripthistory.Sync(ctx, s.db, r.SessionId, decodeTranscriptEvent); err != nil {
		return nil, err
	}
	return transcripthistory.Details(ctx, s.db, r, decodeTranscriptEvent)
}
