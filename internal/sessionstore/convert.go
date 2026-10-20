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
	return len(events), nil
}
