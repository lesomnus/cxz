package workspace

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"time"
)

func (m *Manager) History(ctx context.Context, r *api.WatchRequest) (*api.EventBatch, error) {
	c, client, e := m.ClientFor(ctx, r.SessionId)
	if e == nil {
		q, cancel := context.WithTimeout(ctx, 2*time.Second)
		batch, err := client.History(q, r)
		cancel()
		c.Close()
		if err == nil {
			return batch, m.cache(ctx, batch)
		}
	}
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
