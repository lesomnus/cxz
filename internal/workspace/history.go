package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/convindex"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/historypolicy"
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

func (m *Manager) cache(ctx context.Context, batch *api.EventBatch) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	tx, e := m.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	floors := map[string]uint64{}
	for _, v := range batch.Events {
		if m.removedSessions[v.SessionId] {
			continue
		}
		floor, ok := floors[v.SessionId]
		if !ok {
			floor, e = historypolicy.Floor(ctx, tx, v.SessionId)
			if e != nil {
				return e
			}
		}
		if n := core.HistoryFloor(v.Kind, v.Payload); n > floor {
			if e = historypolicy.AdvanceFloor(ctx, tx, v.SessionId, n); e != nil {
				return e
			}
			floor = n
			marker := &api.Event{SessionId: v.SessionId, RunId: v.RunId, Seq: n, Kind: core.HistoryTrimmedKind, Text: "Earlier display history was removed by the size limit.", Payload: v.Payload}
			data, err := json.Marshal(marker)
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, "INSERT OR REPLACE INTO events VALUES(?,?,?)", v.SessionId, n, data); err != nil {
				return err
			}
		}
		floors[v.SessionId] = floor
	}
	for _, v := range batch.Events {
		if m.removedSessions[v.SessionId] {
			continue
		}
		if v.Seq <= floors[v.SessionId] && (v.Kind != core.HistoryTrimmedKind || core.HistoryFloor(v.Kind, v.Payload) < floors[v.SessionId]) {
			continue
		}
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO events VALUES(?,?,?)", v.SessionId, v.Seq, b); e != nil {
			return e
		}
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	m.extractConversation(ctx, batch)
	return nil
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

// CacheEvents commits streamed events before acknowledging them to the TUI.
func (m *Manager) CacheEvents(ctx context.Context, batch *api.EventBatch) error {
	if err := m.cache(ctx, batch); err != nil {
		return err
	}
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if c, e := m.auxiliaryController(); e == nil {
		var live []*api.Event
		for _, v := range batch.Events {
			if !m.removedSessions[v.SessionId] {
				live = append(live, v)
			}
		}
		c.Observe(live)
	}
	return nil
}
