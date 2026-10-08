package workspace

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/transcripthistory"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func decodeTranscriptEvent(b []byte) (*api.Event, error) {
	var e api.Event
	err := json.Unmarshal(b, &e)
	return &e, err
}
func (m *Manager) Transcript(ctx context.Context, r *api.TranscriptRequest) (*api.TranscriptReply, error) {
	m.refreshHistoryFloor(ctx, r.SessionId)
	c, err := m.historyClient(ctx, r.SessionId)
	if err == nil {
		q, cancel := context.WithTimeout(ctx, 10*time.Second)
		out, e := c.client.Transcript(q, r)
		cancel()
		if e == nil {
			return out, nil
		}
		if code := status.Code(e); code == codes.Unimplemented || code == codes.InvalidArgument || code == codes.NotFound || code == codes.PermissionDenied || code == codes.Unauthenticated {
			return nil, e
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		m.dropHistoryClient(c)
	}
	// Like raw history, the retained local cache remains readable offline. The
	// runtime's index is never copied into the original event cache.
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if err = transcripthistory.Sync(ctx, m.DB, r.SessionId, decodeTranscriptEvent); err != nil {
		return nil, err
	}
	return transcripthistory.Page(ctx, m.DB, r, decodeTranscriptEvent)
}
func (m *Manager) EventDetails(ctx context.Context, r *api.EventDetailsRequest) (*api.EventBatch, error) {
	c, err := m.historyClient(ctx, r.SessionId)
	if err == nil {
		q, cancel := context.WithTimeout(ctx, 10*time.Second)
		out, e := c.client.EventDetails(q, r)
		cancel()
		if e == nil {
			return out, nil
		}
		if code := status.Code(e); code == codes.Unimplemented || code == codes.InvalidArgument || code == codes.NotFound || code == codes.PermissionDenied || code == codes.Unauthenticated {
			return nil, e
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		m.dropHistoryClient(c)
	}
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if err = transcripthistory.Sync(ctx, m.DB, r.SessionId, decodeTranscriptEvent); err != nil {
		return nil, err
	}
	return transcripthistory.Details(ctx, m.DB, r, decodeTranscriptEvent)
}
