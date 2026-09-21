package workspace

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"time"
)

func (m *Manager) History(ctx context.Context, r *api.WatchRequest) (*api.EventBatch, error) {
	// A complete immutable range needs neither Docker inspection nor an RPC.
	cached, err := m.cachedHistory(ctx, r)
	if err != nil {
		return nil, err
	}
	complete := len(cached.Events) == 128
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
	rows, e := m.DB.QueryContext(ctx, "SELECT data FROM events WHERE session_id=? AND seq>? ORDER BY seq LIMIT 128", r.SessionId, r.AfterSeq)
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
	tx, e := m.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, v := range batch.Events {
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO events VALUES(?,?,?)", v.SessionId, v.Seq, b); e != nil {
			return e
		}
	}
	return tx.Commit()
}

// CacheEvents commits streamed events before acknowledging them to the TUI.
func (m *Manager) CacheEvents(ctx context.Context, batch *api.EventBatch) error {
	return m.cache(ctx, batch)
}
