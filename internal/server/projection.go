package server

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
	"github.com/lesomnus/cxz/internal/supervisor"
)

type sessionProjection struct {
	mu         sync.Mutex
	cursor     journal.Cursor
	snapshot   core.Snapshot
	background map[string]*agentview.BackgroundState
}

// lockProjection returns a committed projection with its per-session lock held.
// Only the cursor and reduced state are retained, not the transcript. A failed
// read/transaction never advances them, so the same suffix is retried safely.
func (s *Server) lockProjection(ctx context.Context, m core.Session) (*sessionProjection, error) {
	s.projectionMu.Lock()
	if s.projections == nil {
		s.projections = map[string]*sessionProjection{}
	}
	p := s.projections[m.ID]
	if p == nil {
		p = &sessionProjection{}
		s.projections[m.ID] = p
	}
	s.projectionMu.Unlock()
	p.mu.Lock()
	ok := false
	defer func() {
		if !ok {
			p.mu.Unlock()
		}
	}()
	events, next, reset, err := journal.ReadSince(ctx, filepath.Join(core.Dir(s.root, m.ID), "events.jsonl"), p.cursor)
	if err != nil {
		return nil, err
	}
	var last uint64
	if err = s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq),0) FROM events WHERE session_id=?", m.ID).Scan(&last); err != nil {
		return nil, err
	}
	if last > next.Seq {
		return nil, errors.New("journal is shorter than committed projection; refusing silent data loss")
	}
	if last < p.cursor.Seq && !reset {
		return nil, errors.New("event projection changed unexpectedly; restart to rebuild from journal")
	}
	if last < next.Seq {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		for _, e := range events {
			if e.Seq <= last {
				continue
			}
			b, err := json.Marshal(e)
			if err != nil {
				return nil, err
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO events VALUES(?,?,?)", m.ID, e.Seq, b); err != nil {
				return nil, err
			}
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
	}
	if reset {
		p.snapshot = supervisor.Replay(nil)
		p.background = map[string]*agentview.BackgroundState{}
	}
	p.snapshot = supervisor.ReplayFrom(p.snapshot, events)
	for _, e := range events {
		v := pbEvent(e)
		if v.Kind != "background" {
			continue
		}
		if p.background[e.RunID] == nil {
			p.background[e.RunID] = &agentview.BackgroundState{}
		}
		p.background[e.RunID].Apply(v.Payload)
	}
	p.cursor = next
	ok = true
	return p, nil
}
