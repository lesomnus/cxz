package sessionstore

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/core"
)

// RemoveConvertedJournal requires the caller to hold supervisor.lock. The
// compatibility fence prevents old writers from rebuilding from this residue.
// An oversized/torn/unrecognizable final batch is left for explicit inspection.
func RemoveConvertedJournal(ctx context.Context, dir, session string) (bool, error) {
	done, err := converted(ctx, Path(dir), session)
	if err != nil || !done {
		return false, err
	}
	s, err := Open(ctx, Path(dir), session, false)
	if err != nil {
		return false, err
	}
	defer s.Close()
	compat, err := s.Compat(ctx)
	if err != nil || compat != Compat {
		return false, err
	}
	through, err := s.Through(ctx)
	if err != nil {
		return false, err
	}
	path := filepath.Join(dir, "events.jsonl")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false, err
	}
	if st.Size() > 0 {
		b, err := io.ReadAll(io.NewSectionReader(f, max(0, st.Size()-(1<<20)), min(st.Size(), 1<<20)))
		if err != nil {
			return false, err
		}
		if len(b) == 0 || b[len(b)-1] != '\n' {
			return false, nil
		}
		b = bytes.TrimSuffix(b, []byte{'\n'})
		if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		} else if st.Size() > 1<<20 {
			return false, nil
		}
		var events []core.Event
		if json.Unmarshal(b, &events) != nil {
			var event core.Event
			if json.Unmarshal(b, &event) != nil {
				return false, nil
			}
			events = []core.Event{event}
		}
		if len(events) == 0 {
			return false, nil
		}
		for _, event := range events {
			if event.SessionID != session || event.Seq == 0 || event.Seq > through {
				return false, nil
			}
		}
	}
	if err = ctx.Err(); err != nil {
		return false, err
	}
	fresh, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if !os.SameFile(st, fresh) || st.Size() != fresh.Size() || !st.ModTime().Equal(fresh.ModTime()) {
		return false, nil
	}
	err = os.Remove(path)
	return err == nil, err
}
