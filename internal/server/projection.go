package server

import (
	"context"
	"database/sql"
	"os"
	"sync"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/eventview"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"github.com/lesomnus/cxz/internal/observe"
	"github.com/lesomnus/cxz/internal/sessionstore"
	"github.com/lesomnus/cxz/internal/supervisor"
	"github.com/lesomnus/cxz/internal/transcripthistory"
)

type sessionProjection struct {
	mu       sync.Mutex
	floor    uint64
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
	// derived says the reduced state above was built from this store, so a call
	// that finds the session unmoved can answer without building it again.
	derived bool
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
	// The session's own store is the conversation. It records its reduced state
	// beside its records, so a projection new to this process starts where the
	// session left off instead of replaying anything -- which on a real
	// installation was 0.26 to 0.95s per session, every time this process was
	// new.
	//
	// Until stage 5 this server also read the journal, kept a copy of the
	// events in its own database, and used a fence -- the journal's size and
	// mtime -- to decide which of the two to trust. There is no journal to
	// read, so there is nothing to reconcile and nothing to copy.
	// See cxz#278.
	if p.store == nil {
		// A session with no store has recorded nothing: it was created and has
		// not started, or it is being asked about while its directory is still
		// being made. That is an empty conversation, not an error.
		if _, err := os.Stat(sessionstore.Path(core.Dir(s.root, m.ID))); err != nil {
			p.snapshot = supervisor.Replay(nil)
			p.background = map[string]*agentview.BackgroundState{}
			p.catalogs = map[string]catalogRecord{}
			ok = true
			return p, nil
		}
		store, err := sessionstore.Open(ctx, sessionstore.Path(core.Dir(s.root, m.ID)), m.ID, false)
		if err != nil {
			return nil, err
		}
		p.store = store
	}
	snap, found, err := p.store.State(ctx)
	if err != nil {
		return nil, err
	}
	if !found {
		through, err := p.store.Through(ctx)
		if err != nil {
			return nil, err
		}
		if through == 0 {
			// A session that has recorded nothing yet. Not an error: it is about to.
			p.snapshot = supervisor.Replay(nil)
			p.background = map[string]*agentview.BackgroundState{}
			p.catalogs = map[string]catalogRecord{}
			ok = true
			return p, nil
		}
		// Records and no state: a session that last ran before it wrote its
		// state beside its records, and has not run since. Its state is what
		// its records reduce to -- the same replay its supervisor does when it
		// launches, and the one that writes the state down. Reading it as
		// empty answered with no run, so resuming it was refused as stale
		// before the supervisor that would have fixed it could start.
		// Nothing is recording, so the replay is done once per process.
		if p.derived && p.snapshot.LastSeq == through {
			snap = p.snapshot
		} else {
			events, err := p.store.Events(ctx)
			if err != nil {
				return nil, err
			}
			snap = supervisor.Replay(events)
		}
	}
	// Derived on every call where the session has moved, not once: a session
	// keeps recording, and a projection taken and then left alone would answer
	// with the conversation as it was when it was taken -- which is how the
	// first version of this left `Background` empty for the rest of a run.
	if !p.derived || snap.LastSeq != p.snapshot.LastSeq {
		if err = s.deriveFromStore(ctx, p, m); err != nil {
			return nil, err
		}
		p.derived = true
	}
	p.snapshot = snap
	ok = true
	return p, nil
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
	// Folded by the writer, because a raw background frame gets no row in the
	// reading projection and finding it again would mean walking the whole
	// conversation. See cxz#278.
	background, err := p.store.Background(ctx)
	if err != nil {
		return err
	}
	p.background = background
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

// conversation is the database a session's conversation is read from, which is
// the session's own store. It used to be this or the server's copy of the
// events, decided by a fence against the journal; there is neither a journal
// nor a copy. See cxz#278.
func (s *Server) conversation(p *sessionProjection) *sql.DB {
	if p.store == nil {
		return nil // a session that has recorded nothing
	}
	return p.store.DB()
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
	store, err := sessionstore.Open(ctx, sessionstore.Path(core.Dir(s.root, m.ID)), m.ID, false)
	if err != nil {
		return s.db, func() {}
	}
	return store.DB(), func() { store.Close() }
}
