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
	// In batches, because one transaction over a 122k-record journal holds a
	// write lock for as long as it takes and this runs while the session may be
	// starting. The reduction after each batch is incremental, so the cost is
	// the batch rather than the conversation.
	const batch = 2048
	for i := 0; i < len(events); i += batch {
		end := min(i+batch, len(events))
		if err = s.Append(ctx, events[i:end]); err != nil {
			return i, fmt.Errorf("convert %s at %d: %w", session, i, err)
		}
	}
	return len(events), nil
}
