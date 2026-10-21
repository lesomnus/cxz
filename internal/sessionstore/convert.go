package sessionstore

import (
	"context"
	"fmt"
	"os"

	"path/filepath"

	"github.com/lesomnus/cxz/internal/journal"
)

// Convert builds a session's store from the journal it already has. It is how a
// session that predates the store gets one, and it is re-runnable: a record
// already stored is written again with the same bytes, so an interrupted
// conversion is finished by the next attempt rather than repaired.
//
// The journal stays where it is. While it is still the truth -- stages 1 to 4
// of docs/plans/session-conversation-store.md -- this store is derived, and a
// conversion that fails leaves a session reading exactly what it read before.
func Convert(ctx context.Context, dir, session string) (int, error) {
	path := Path(dir)
	// Once. Reading a journal is reading all of it -- 82MB on this installation
	// for one session -- and after stage 5 nothing appends to it, so a store
	// that says it has been converted is the whole record. The mark is written
	// inside the store, so an interrupted conversion has no mark and the next
	// attempt finishes it. See cxz#278.
	if done, err := converted(ctx, path, session); err != nil {
		return 0, err
	} else if done {
		return 0, nil
	}
	events, _, _, err := journal.ReadSince(ctx, filepath.Join(dir, "events.jsonl"), journal.Cursor{})
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	s, err := Open(ctx, path, session, true)
	if err != nil {
		return 0, err
	}
	defer s.Close()
	through, err := s.Through(ctx)
	if err != nil {
		return 0, err
	}
	n, err := s.Fill(ctx, events, through)
	if err != nil {
		return n, fmt.Errorf("convert %s at %d: %w", session, n, err)
	}
	if _, err = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO meta VALUES('converted','1')`); err != nil {
		return n, err
	}
	return len(events), nil
}

// converted reports whether a store already holds everything its journal had.
// A store that does not exist yet has not, and says so without creating one.
func converted(ctx context.Context, path, session string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	s, err := Open(ctx, path, session, false)
	if err != nil {
		return false, err
	}
	defer s.Close()
	n, err := s.number(ctx, "converted")
	return n == 1, err
}
