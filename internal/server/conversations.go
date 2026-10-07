package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/convindex"
	"github.com/lesomnus/cxz/internal/core"
)

// Keeping the conversation searchable is the write path's job, not a reader's.
//
// Every event this daemon commits passes through one place already, so that is
// where the conversation is extracted. The index is derived: it is rebuilt from
// what this installation stores, it is caught up before a search rather than
// trusted blindly, and what it has not reached is reported instead of silently
// missing from an answer.
const (
	// catchUpBudget bounds the work a search will do before answering. A search
	// is not the place to rebuild an index; it is the place to notice that one
	// is behind and say so.
	catchUpBudget = 3 * time.Second
	ingestBatch   = 512
)

// A project runtime keeps no index: its conversation belongs to the
// installation that owns it, and searching from inside a project is refused.
func (s *Server) conversationsEnabled() bool { return !s.insideProject() }

func (s *Server) openConversations(ctx context.Context) error {
	if !s.conversationsEnabled() {
		return nil
	}
	index, err := convindex.Open(ctx, s.root)
	if err != nil {
		return err
	}
	s.conversations = index
	return nil
}

// ingestConversation records what a batch of committed events said. It is
// called where events are committed, so the index is current for anything
// happening now; history from before it existed is the seed's job.
func (s *Server) ingestConversation(ctx context.Context, m core.Session, events []core.Event, from, through uint64) {
	if s.conversations == nil || (len(events) == 0 && through == 0) {
		return
	}
	session := convindex.Session{ID: m.ID, Project: m.ProjectID, Title: m.Title, Agent: m.Kind, CreatedMS: m.CreatedAt}
	if session.Agent == "" {
		session.Agent = m.Agent
	}
	if err := s.conversations.Ingest(ctx, session, events, from, through); err != nil {
		// A derived store failing must not fail the thing it was derived from.
		fmt.Fprintln(os.Stderr, "conversation index:", err)
	}
}

func (s *Server) forgetConversation(ctx context.Context, id string) {
	if s.conversations == nil {
		return
	}
	if err := s.conversations.Forget(ctx, id); err != nil {
		fmt.Fprintln(os.Stderr, "conversation index:", err)
	}
}

// Seed builds the index from what this installation has already stored, once,
// in the background. A search catches up on what it needs within a budget, but
// a fresh or deleted index should not be rebuilt a search at a time: it is
// derived, so rebuilding it is always allowed and never urgent.
func (s *Server) Seed(ctx context.Context) {
	if s.conversations == nil {
		return
	}
	start := time.Now()
	read := 0
	for _, session := range s.conversationSessions(ctx) {
		if ctx.Err() != nil {
			return
		}
		cursor, err := s.conversations.Cursor(ctx, session.ID)
		if err != nil {
			continue
		}
		n, _, err := s.ingestStored(ctx, session, cursor)
		if err != nil {
			fmt.Fprintln(os.Stderr, "conversation index:", err)
			continue
		}
		read += n
	}
	if read > 0 {
		fmt.Fprintf(os.Stderr, "conversation index: %d events in %s\n", read, time.Since(start).Round(time.Millisecond))
	}
}

// Pending is a session the index has not caught up with, named in the reply so
// that an incomplete answer says which part is missing.
type Pending struct {
	Project, Name, State, Message string
	Done, Total                   int
}

// catchUp brings the index level with the events this installation has already
// stored, which is what makes it correct after an upgrade and after any gap.
//
// It reads the local event store rather than the journals: the journals live in
// each project's own volume, and the point of the index is that answering a
// question does not open them. A session whose events have not reached this
// store yet -- a project nobody has watched since it last spoke -- is reported
// rather than fetched, because a search is not the place to wait for one.
func (s *Server) catchUp(ctx context.Context) []Pending {
	if s.conversations == nil {
		return nil
	}
	budget := s.catchUpBudget
	if budget == 0 {
		budget = catchUpBudget
	}
	deadline, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	var pending []Pending
	for _, session := range s.conversationSessions(ctx) {
		if deadline.Err() != nil {
			pending = append(pending, Pending{Project: session.ProjectID, Name: session.Title, State: "behind",
				Message: "not caught up before this search answered"})
			continue
		}
		cursor, err := s.conversations.Cursor(deadline, session.ID)
		if err != nil {
			continue
		}
		read, last, err := s.ingestStored(deadline, session, cursor)
		if err != nil {
			pending = append(pending, Pending{Project: session.ProjectID, Name: session.Title, State: "unavailable", Message: err.Error()})
			continue
		}
		if read > 0 {
			pending = append(pending, Pending{Project: session.ProjectID, Name: session.Title, State: "indexed",
				Message: fmt.Sprintf("%d events", read), Done: int(last), Total: int(last)})
		}
	}
	return pending
}

// ingestStored copies this installation's stored events for one session into
// the index, from where the index stopped.
func (s *Server) ingestStored(ctx context.Context, m core.Session, cursor uint64) (int, uint64, error) {
	read := 0
	last := cursor
	for {
		rows, err := s.db.QueryContext(ctx, "SELECT seq, data FROM events WHERE session_id=? AND seq>? ORDER BY seq LIMIT ?", m.ID, last, ingestBatch)
		if err != nil {
			return read, last, err
		}
		var batch []core.Event
		highest := last
		for rows.Next() {
			var seq uint64
			var data []byte
			if err = rows.Scan(&seq, &data); err != nil {
				rows.Close()
				return read, last, err
			}
			e, ok := storedEvent(data, m.ID, seq)
			if ok {
				batch = append(batch, e)
			}
			highest = max(highest, seq)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return read, last, err
		}
		if highest == last {
			return read, last, nil
		}
		s.ingestConversation(ctx, m, batch, last+1, highest)
		read += len(batch)
		last = highest
		if ctx.Err() != nil {
			return read, last, nil
		}
	}
}

// storedEvent decodes one row of the event store. The manager and a host-local
// daemon write the same table from different types -- the manager stores the
// view model it streams, a daemon stores the journal's own event -- and the two
// agree on every field this index reads except how a payload is encoded.
func storedEvent(data []byte, session string, seq uint64) (core.Event, bool) {
	var e core.Event
	if json.Unmarshal(data, &e) == nil && e.Kind != "" {
		e.SessionID, e.Seq = session, seq
		return e, true
	}
	var v api.Event
	if json.Unmarshal(data, &v) != nil || v.Kind == "" {
		return e, false
	}
	return core.Event{SessionID: session, RunID: v.RunId, Seq: seq, TimeMS: v.TimeMs, Kind: v.Kind, Text: v.Text, Payload: v.Payload}, true
}

// conversationSessions lists what the index should know about, newest first so
// that a bounded catch-up spends its time where a search is most likely to look.
func (s *Server) conversationSessions(ctx context.Context) []core.Session {
	if s.manager != nil {
		return s.manager.ConversationSessions(ctx)
	}
	out, err := s.list(ctx)
	if err != nil {
		return nil
	}
	return out
}
