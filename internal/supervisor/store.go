package supervisor

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
	"github.com/lesomnus/cxz/internal/sessionstore"
)

// The store is this session's conversation in a file the session owns, and
// while the journal is still written it is derived from it. That is the whole
// contract here: a journal append that fails is a durability failure and kills
// the run, and a store write that fails is not. It is reported, the store is
// marked as owing a rebuild, and the conversation carries on being recorded
// where it has always been recorded.
//
// See docs/plans/session-conversation-store.md.

// openStore prepares this session's store, converting the journal into it when
// it has none yet. A conversion that fails leaves the session exactly as it was:
// reading still comes from the journal until the server is switched over.
func (s *Supervisor) openStore(ctx context.Context, dir string) {
	store, err := sessionstore.Open(ctx, sessionstore.Path(dir), s.session.ID, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "conversation store:", err)
		return
	}
	// A store that has fewer records than the journal is one that has never been
	// built or was interrupted. Converting is re-runnable, so the cheap thing is
	// to ask and the correct thing is to finish.
	stored, err := store.Through(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "conversation store:", err)
		store.Close()
		return
	}
	all := s.log.All()
	last := uint64(0)
	if len(all) > 0 {
		last = all[len(all)-1].Seq
	}
	if stored < last {
		start := time.Now()
		n, err := store.Fill(ctx, all, stored)
		if err != nil {
			fmt.Fprintln(os.Stderr, "conversation store: convert", s.session.ID, err)
			store.Close()
			return
		}
		fmt.Fprintf(os.Stderr, "conversation store: %s converted %d records through %d in %v\n",
			s.session.ID, n, last, time.Since(start).Round(time.Millisecond))
	}
	s.store = store
	s.recordState(ctx)
}

func (s *Supervisor) closeStore() {
	if s.store == nil {
		return
	}
	_ = s.store.Close()
	s.store = nil
}

// record puts what the journal just accepted into the store. The events it is
// given are the ones the journal returned, so they carry the sequence and time
// it assigned -- the store holds what the journal holds, not an approximation
// of it.
func (s *Supervisor) record(events []core.Event) {
	if s.store == nil || len(events) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.store.Append(ctx, events); err != nil {
		// Derived, so this is not a durability failure. Saying so is the whole
		// response: the next append or the next start finishes what this did not.
		fmt.Fprintln(os.Stderr, "conversation store:", err)
		return
	}
	s.recordState(ctx)
}

// recordState writes the reduced state beside the records, fenced to the
// journal it was reduced from. A reader that finds the fence still matching has
// the state without replaying the journal; one that does not replays, which is
// what every reader did before this.
//
// It is written after the records, never before: a fence that claims more than
// the store holds would let a reader skip records that are there.
func (s *Supervisor) recordState(ctx context.Context) {
	size, mtime, err := journal.Fence(s.log.Path())
	if err != nil {
		fmt.Fprintln(os.Stderr, "conversation store:", err)
		return
	}
	if err = s.store.SetState(ctx, s.snap, sessionstore.Fence{Size: size, ModTime: mtime, Seq: s.snap.LastSeq}); err != nil {
		fmt.Fprintln(os.Stderr, "conversation store:", err)
	}
}
