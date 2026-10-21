package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionstore"
)

// ConvertStores gives every session the manager knows a store of its own.
//
// A conversion runs when a supervisor starts, and reading a conversation
// converts it (cxz#268), but a session that stopped before stores existed and
// that nobody opens again would stay a journal alone. That is the one thing
// that has to be untrue before the journal can stop being written: stage 5 of
// docs/plans/session-conversation-store.md makes the store the only record, and
// a session whose record is only in a file nobody writes any more is a
// conversation nobody can read.
//
// In the background, never on the startup path. Converting 82MB of journal is
// seconds of work, and a manager that is still doing it when the rollout asks
// whether it is healthy gets rolled back -- which is exactly how cxz#272's
// first attempt was lost.
func (m *Manager) ConvertStores(ctx context.Context) {
	if m.StateVolume == "" {
		return
	}
	start := time.Now()
	converted, records := 0, 0
	projects, err := m.all(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "conversation stores:", err)
		return
	}
	for _, p := range projects {
		dir, _ := m.projectState(p.ID)
		for _, s := range p.Sessions {
			if ctx.Err() != nil {
				return
			}
			sessionDir := core.Dir(filepath.Join(dir, "data"), s.Id)
			if _, err := os.Stat(sessionstore.Path(sessionDir)); err == nil {
				continue
			}
			if _, err := os.Stat(filepath.Join(sessionDir, "events.jsonl")); err != nil {
				continue // nothing was ever recorded for this session
			}
			n, err := sessionstore.Convert(ctx, sessionDir, s.Id)
			if err == nil {
				// This pass runs in the manager, which is root. A store it
				// leaves owned by root is one the session's own runtime cannot
				// open. See cxz#280.
				err = ownLikeDir(sessionDir, storeFiles(sessionstore.Path(sessionDir))...)
			}
			if err != nil {
				// One session that cannot be converted is reported and left
				// alone: its journal is still there, and the next pass tries
				// again.
				fmt.Fprintf(os.Stderr, "conversation store for %s: %v\n", s.Id, err)
				continue
			}
			converted++
			records += n
		}
	}
	if converted > 0 {
		fmt.Fprintf(os.Stderr, "converted %d conversations (%d records) in %s\n", converted, records, time.Since(start).Round(time.Millisecond))
	}
}
