package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/convindex"
	"github.com/lesomnus/cxz/internal/core"
)

// History is a page of a session's conversation, read from the session's own
// store. The manager used to keep a mirror of every page in its own database
// and serve that when a project could not answer; the store is a file in the
// manager's volume now, so there is nothing to mirror and nothing to fall back
// to. See cxz#268.
func (m *Manager) History(ctx context.Context, r *api.WatchRequest) (*api.EventBatch, error) {
	store, err := m.conversationStore(ctx, r.SessionId)
	if errors.Is(err, errNoConversation) {
		return &api.EventBatch{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return storedHistory(ctx, store.DB(), r)
}

// extractConversation keeps the searchable record level with the events as they
// are committed, which is what makes a search a query rather than a scan. It
// cannot fail the commit: the index is derived, and a derived store that falls
// behind is reported by the search it serves.
func (m *Manager) extractConversation(ctx context.Context, batch *api.EventBatch) {
	if m.Conversations == nil {
		return
	}
	bySession := map[string][]core.Event{}
	for _, v := range batch.Events {
		if v.SessionId == "" || v.Seq == 0 {
			continue
		}
		bySession[v.SessionId] = append(bySession[v.SessionId], core.Event{
			SessionID: v.SessionId, RunID: v.RunId, Seq: v.Seq, TimeMS: v.TimeMs,
			Kind: v.Kind, Text: v.Text, Payload: v.Payload,
		})
	}
	for id, events := range bySession {
		if err := m.Conversations.Ingest(ctx, convindex.Session{ID: id}, events, 0, 0); err != nil {
			fmt.Fprintln(os.Stderr, "conversation index:", err)
		}
	}
}

// CacheEvents is what the manager does with events on their way from a project
// runtime to a client: it keeps the searchable record level with them and shows
// them to the auxiliary agent. It no longer keeps them -- the session's own
// store already has them, which is why the name is now the only thing left of
// the cache. See cxz#268.
func (m *Manager) CacheEvents(ctx context.Context, batch *api.EventBatch) error {
	m.writeMu.Lock()
	live := &api.EventBatch{}
	for _, v := range batch.Events {
		// A session that was removed can still have frames in flight, and
		// neither the index nor a summary should take them.
		if !m.removedSessions[v.SessionId] {
			live.Events = append(live.Events, v)
		}
	}
	c, e := m.auxiliaryController()
	m.writeMu.Unlock()
	m.extractConversation(ctx, live)
	if e == nil {
		c.Observe(live.Events)
	}
	return nil
}
