package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/convindex"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/historypage"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"time"
)

func (m *Manager) History(ctx context.Context, r *api.WatchRequest) (*api.EventBatch, error) {
	// Refresh the retention floor at most once a minute; old cached pages must
	// not outlive a missed/offline trim notification.
	m.refreshHistoryFloor(ctx, r.SessionId)
	// Complete retained ranges still reuse their cached payload.
	cached, err := m.cachedHistory(ctx, r)
	if err != nil {
		return nil, err
	}
	complete := len(cached.Events) == historypage.PageSize(r.Limit)
	for i, e := range cached.Events {
		complete = complete && e.Seq == r.AfterSeq+uint64(i)+1
	}
	if complete {
		return cached, nil
	}
	c, e := m.historyClient(ctx, r.SessionId)
	if e == nil {
		q, cancel := context.WithTimeout(ctx, 2*time.Second)
		batch, err := c.client.History(q, r)
		cancel()
		if err == nil {
			return batch, m.cache(ctx, batch)
		}
		if ctx.Err() == nil {
			m.dropHistoryClient(c)
		}
	}
	return cached, nil
}

func (m *Manager) cachedHistory(ctx context.Context, r *api.WatchRequest) (*api.EventBatch, error) {
	rows, e := m.DB.QueryContext(ctx, "SELECT data FROM events WHERE session_id=? AND seq>? ORDER BY seq LIMIT ?", r.SessionId, r.AfterSeq, historypage.PageSize(r.Limit))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := &api.EventBatch{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		var v api.Event
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		out.Events = append(out.Events, &v)
	}
	return out, rows.Err()
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

// A small metadata RPC keeps caches consistent even when no TUI was attached at
// the moment of compaction. Failure leaves offline cached history readable.
func (m *Manager) refreshHistoryFloor(ctx context.Context, id string) {
	c, err := m.historyClient(ctx, id)
	if err != nil {
		return
	}
	m.historyMu.Lock()
	if c.checked == nil {
		c.checked = map[string]time.Time{}
	}
	if time.Since(c.checked[id]) < time.Minute {
		m.historyMu.Unlock()
		return
	}
	c.checked[id] = time.Now()
	m.historyMu.Unlock()
	spec, _ := json.Marshal(&api.SessionRef{Id: id})
	q, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	reply, err := c.client.Docker(q, &api.DockerInput{Action: "history-floor", Spec: spec})
	if err != nil || reply == nil {
		return
	}
	var boundary core.HistoryBoundary
	if json.Unmarshal([]byte(reply.Status), &boundary) != nil || boundary.Through == 0 {
		return
	}
	payload, _ := json.Marshal(boundary)
	_ = m.cache(ctx, &api.EventBatch{Events: []*api.Event{{SessionId: id, Seq: boundary.Through, Kind: core.HistoryTrimmedKind, Payload: payload}}})
}
