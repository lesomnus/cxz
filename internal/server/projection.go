package server

import (
	"bytes"
	"context"
	"database/sql"
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
	"github.com/lesomnus/cxz/internal/sessionstore"
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
	// store is the session's own conversation, opened for reading. The session
	// writes it; this server reads it. Held open because opening it per call
	// would be a file open per page.
	store *sessionstore.Store
	// owned says this projection was taken from what the session recorded, so
	// the journal has not been read in this process and the rows belong to the
	// session rather than to this server.
	owned bool
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
	path := filepath.Join(core.Dir(s.root, m.ID), "events.jsonl")
	// A session records its reduced state beside its records, fenced to the
	// journal it was reduced from. When that fence still matches, the state is
	// this state and the journal is not read at all -- which on a real
	// installation was 0.26 to 0.95s per session every time this process was
	// new. See docs/plans/session-conversation-store.md.
	taken, err := s.takeRecorded(ctx, p, m, path)
	if err != nil {
		return nil, err
	}
	if taken {
		ok = true
		return p, nil
	}
	events, next, reset, err := journal.ReadSince(ctx, path, p.cursor)
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

// takeRecorded takes a session's own reduced state when it can be trusted, so
// that a projection new to this process starts where the session left off
// rather than replaying its journal.
//
// Trust is the fence and nothing else: the journal's size and modification time
// as they were when the session wrote the state. The journal is only appended
// to, and compaction replaces the file, so a match means nothing has been
// recorded since. Anything else -- no store, no state, a fence that no longer
// matches -- answers false, and the caller reads the journal exactly as it
// always has.
//
// The rest of what a projection holds is derived here rather than persisted,
// because the store indexes its records by kind: the newest model catalog of
// each run is one indexed look, background state is the records of one kind
// folded in order, and the trim floor is already durable in the policy table.
// Nothing new has to be written for any of them.
func (s *Server) takeRecorded(ctx context.Context, p *sessionProjection, m core.Session, path string) (bool, error) {
	if !p.owned && p.cursor.Seq > 0 {
		return false, nil // this process is already reading the journal
	}
	if p.store == nil {
		store, err := sessionstore.Open(ctx, sessionstore.Path(core.Dir(s.root, m.ID)), m.ID, false)
		if err != nil {
			return false, nil // no store yet; the journal is still the only truth
		}
		p.store = store
	}
	snap, fence, found, err := p.store.State(ctx)
	if err != nil || !found {
		return false, nil
	}
	size, mtime, err := journal.Fence(path)
	if err != nil {
		return false, nil
	}
	if fence.Size != size || fence.ModTime != mtime || fence.Seq != snap.LastSeq {
		return false, nil
	}
	through, err := p.store.Through(ctx)
	if err != nil || through != fence.Seq {
		return false, nil
	}
	// Checked on every call, not once. A session keeps recording, so a
	// projection taken and then left alone would answer with the conversation as
	// it was when it was taken -- which is how the first version of this left
	// `Background` empty for the rest of a run.
	if !p.owned || snap.LastSeq != p.snapshot.LastSeq {
		if err = s.deriveFromStore(ctx, p, m); err != nil {
			return false, err
		}
	}
	p.snapshot = snap
	p.cursor = journal.Cursor{Offset: size, Seq: snap.LastSeq}
	p.owned = true
	return true, nil
}

// deriveFromStore fills what a journal replay used to leave behind, with
// indexed reads instead of a walk.
func (s *Server) deriveFromStore(ctx context.Context, p *sessionProjection, m core.Session) error {
	catalogs, err := p.store.LatestPerRun(ctx, "models")
	if err != nil {
		return err
	}
	p.catalogs = map[string]catalogRecord{}
	for _, e := range catalogs {
		v := eventview.FromCore(e)
		p.catalogs[e.RunID] = catalogRecord{payload: v.Payload, seq: v.Seq, ms: v.TimeMs}
	}
	background, err := p.store.EachOfKind(ctx, "background")
	if err != nil {
		return err
	}
	p.background = map[string]*agentview.BackgroundState{}
	for _, e := range background {
		v := eventview.FromCore(e)
		if p.background[e.RunID] == nil {
			p.background[e.RunID] = &agentview.BackgroundState{}
		}
		p.background[e.RunID].Apply(v.Payload)
	}
	// The trim floor is already durable, in the policy table this server owns.
	// It is read in a transaction because that is what the policy helper takes,
	// and it creates its table on the way past.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	floor, err := historypolicy.Floor(ctx, tx, m.ID)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	p.floor = floor
	return nil
}

// conversation is the database a session's conversation is read from: the
// session's own store once this server has taken that store's state, and the
// server's cache until then. A session is in one or the other and never half of
// each, because the fence that decides it is the same fence that decided
// whether the journal was read.
func (s *Server) conversation(p *sessionProjection) *sql.DB {
	if p.owned && p.store != nil {
		return p.store.DB()
	}
	return s.db
}

// conversationFor is `conversation` for a caller that does not hold the
// projection and must not take it. The projection is the runtime's; a manager
// runs this same server with no journals at all, so asking for a projection
// here would fail for every session it holds.
//
// The store it answers with is the caller's to close.
func (s *Server) conversationFor(ctx context.Context, m core.Session) (*sql.DB, func()) {
	if s.manager != nil {
		// Running beside the manager, which has no journals of its own: every
		// session's store is a file in the manager's volume, and the manager
		// knows which. It used to read the manager's copy of the events, which
		// stops existing in cxz#268.
		store, err := s.manager.ConversationStore(ctx, m.ID)
		if err != nil {
			return s.db, func() {}
		}
		return store.DB(), func() { store.Close() }
	}
	path := filepath.Join(core.Dir(s.root, m.ID), "events.jsonl")
	size, mtime, err := journal.Fence(path)
	if err != nil {
		return s.db, func() {} // no journal here: this is the cache's session
	}
	store, err := sessionstore.Open(ctx, sessionstore.Path(core.Dir(s.root, m.ID)), m.ID, false)
	if err != nil {
		return s.db, func() {}
	}
	_, fence, found, err := store.State(ctx)
	if err != nil || !found || fence.Size != size || fence.ModTime != mtime {
		store.Close()
		return s.db, func() {}
	}
	through, err := store.Through(ctx)
	if err != nil || through != fence.Seq {
		store.Close()
		return s.db, func() {}
	}
	return store.DB(), func() { store.Close() }
}
