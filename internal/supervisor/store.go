package supervisor

import (
	"context"
	"fmt"
	"os"
	"sort"
)

// The store is this session's conversation, in a file the session owns. It is
// the record now: the journal is no longer written, so there is nothing to
// derive from and nothing to fall back to. A write that fails is a durability
// failure, which is why it is the log that panics, exactly as the journal did.
//
// See docs/plans/session-conversation-store.md.

// recordState writes the reduced state beside the records, after them and never
// before. A reader finds the state without replaying anything.
//
// It used to carry a fence -- the journal's size and mtime -- so a reader could
// tell whether this file was behind the record. This file is the record.
// See cxz#278.
func (s *Supervisor) recordState(ctx context.Context) {
	if s.store == nil {
		return
	}
	// With the pending approvals, because a reader of this state must see what
	// a replay of the records would have told it: Replay collects them, the
	// live snapshot keeps them in a map of its own, and a reader that got the
	// snapshot without them saw a session waiting for an answer as idle.
	snap := s.snap
	snap.Pending = nil
	for _, e := range s.pending {
		snap.Pending = append(snap.Pending, e)
	}
	sort.Slice(snap.Pending, func(i, j int) bool { return snap.Pending[i].Seq < snap.Pending[j].Seq })
	if err := s.store.SetState(ctx, snap); err != nil {
		fmt.Fprintln(os.Stderr, "conversation store:", err)
	}
}
