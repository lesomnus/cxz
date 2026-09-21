package server

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

func (s *Server) History(ctx context.Context, r *api.WatchRequest) (*api.EventBatch, error) {
	if s.manager != nil {
		return s.manager.History(ctx, r)
	}
	m, e := s.manifest(ctx, r.SessionId)
	if e != nil {
		return nil, e
	}
	p, e := s.lockProjection(ctx, m)
	if e != nil {
		return nil, e
	}
	p.mu.Unlock()
	rows, e := s.db.QueryContext(ctx, "SELECT data FROM events WHERE session_id=? AND seq>? ORDER BY seq LIMIT 128", r.SessionId, r.AfterSeq)
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
		var v core.Event
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		out.Events = append(out.Events, pbEvent(v))
	}
	return out, rows.Err()
}
