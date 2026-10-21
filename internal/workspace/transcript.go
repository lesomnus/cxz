package workspace

import (
	"context"
	"errors"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/sessionstore"
	"github.com/lesomnus/cxz/internal/transcripthistory"
)

// Transcript is the reading projection of a session's conversation, read from
// the session's own store. The rows are built by the writer: `sessionstore`
// syncs them on every append, which is why a reader needs no write access and
// why the project does not have to be running. See cxz#268.
func (m *Manager) Transcript(ctx context.Context, r *api.TranscriptRequest) (*api.TranscriptReply, error) {
	store, err := m.conversationStore(ctx, r.SessionId)
	if errors.Is(err, errNoConversation) {
		return &api.TranscriptReply{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return transcripthistory.Page(ctx, store.DB(), r, sessionstore.Decode)
}

// EventDetails is one event's full record, read from the same store. A reader
// that asks for an event the session no longer retains is told so by the
// projection, as before. See cxz#268.
func (m *Manager) EventDetails(ctx context.Context, r *api.EventDetailsRequest) (*api.EventBatch, error) {
	store, err := m.conversationStore(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return transcripthistory.Details(ctx, store.DB(), r, sessionstore.Decode)
}
