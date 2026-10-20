package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/lesomnus/cxz/internal/eventview"
	"path/filepath"
	"sync"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"github.com/lesomnus/cxz/internal/journal"
	"github.com/lesomnus/cxz/internal/observe"
	"github.com/lesomnus/cxz/internal/supervisor"
	"github.com/lesomnus/cxz/internal/transcripthistory"
)

type sessionProjection struct {
	mu         sync.Mutex
	cursor     journal.Cursor
	floor      uint64
	snapshot   core.Snapshot
	background map[string]*agentview.BackgroundState
	// catalogs is the latest model catalog per run. It is reduced state like the
	// background map: clients ask what the current capability is, which is one
	// event out of a journal, and reconstructing that by reading the journal from
	// a client costs the whole conversation to find one record.
	catalogs map[string]catalogRecord
	// transcript is the reading projection's reduced state, kept here because
	// this lock is what serializes a session: without it every sync rebuilds
	// that state by reading every current row of the conversation. Only a
	// caller holding mu may pass it on.
	transcript transcripthistory.Cache
}

type catalogRecord struct {
	payload []byte
	seq     uint64
	ms      int64
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
	observe.Wait(ctx, "session projection", p.mu.Lock)
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
	var floor uint64
	if reset && len(events) > 0 && events[0].Kind == core.HistoryCheckpointKind {
		floor = events[0].Seq
	}
	if last < next.Seq || floor > 0 {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if floor > 0 {
			if err = historypolicy.AdvanceFloor(ctx, tx, m.ID, floor); err != nil {
				return nil, err
			}
		}

		for _, e := range events {
			if e.Seq <= last && e.Seq != floor {
				continue
			}
			b, err := json.Marshal(e)
			if err != nil {
				return nil, err
			}
			// The checkpoint at the floor is written even though its sequence is
			// behind the cache, because it replaces the record that compaction
			// removed. Once it has, writing it again is writing the same bytes --
			// and a replacement is a new rowid, which is how the reading
			// projection tells a backfill from an append. So every restart of
			// this process, whose first look at a journal is always a reset, used
			// to put a record behind every reader's cursor and force a session to
			// be projected from the beginning: 4s on a 103k-record conversation,
			// 18s on a 146k one, paid by whoever opened it first. See cxz#227.
			if e.Seq <= last {
				var stored []byte
				if err = tx.QueryRowContext(ctx, "SELECT data FROM events WHERE session_id=? AND seq=?", m.ID, e.Seq).Scan(&stored); err == nil && bytes.Equal(stored, b) {
					continue
				}
			}
			if _, err = tx.ExecContext(ctx, "INSERT OR REPLACE INTO events VALUES(?,?,?)", m.ID, e.Seq, b); err != nil {
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
		p.catalogs = map[string]catalogRecord{}
	}
	p.snapshot = supervisor.ReplayFrom(p.snapshot, events)
	for _, e := range events {
		if e.Kind == core.HistoryCheckpointKind {
			var checkpoint core.HistoryCheckpoint
			if err := json.Unmarshal(e.Payload, &checkpoint); err != nil {
				return nil, err
			}
			for _, saved := range checkpoint.Resume {
				if saved.Kind == "background" {
					if p.background[saved.RunID] == nil {
						p.background[saved.RunID] = &agentview.BackgroundState{}
					}
					p.background[saved.RunID].Apply(saved.Payload)
				}
			}
		}
		v := eventview.FromCore(e)
		if v.Kind == "models" {
			if p.catalogs == nil {
				p.catalogs = map[string]catalogRecord{}
			}
			// Last one wins: a run republishes after a setting is applied, and
			// only the newest describes the agent now.
			p.catalogs[e.RunID] = catalogRecord{payload: v.Payload, seq: v.Seq, ms: v.TimeMs}
			continue
		}
		if v.Kind != "background" {
			continue
		}
		if p.background[e.RunID] == nil {
			p.background[e.RunID] = &agentview.BackgroundState{}
		}
		p.background[e.RunID].Apply(v.Payload)
	}
	if floor > 0 {
		p.floor = floor
	}
	p.cursor = next
	ok = true
	return p, nil
}
