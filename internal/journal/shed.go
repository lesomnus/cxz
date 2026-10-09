package journal

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/core"
)

// Shed empties the verbatim vendor payload of every raw event at or below
// through, and answers with the bytes that are no longer on disk.
//
// It empties rather than removes. An event that is gone and an event that was
// never written look the same to a reader, and the sequence it occupied would
// become a hole in a file whose numbering is how everything else finds its
// place; a raw event with nothing in it still says a turn produced one, and the
// one consumer of the payload -- the raw view -- already skips an empty one.
//
// The cost of keeping the stub is a few dozen bytes against the kilobytes to
// megabytes the payload was.
func (l *Log) Shed(through uint64) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var freed int64
	keep := make([]core.Event, 0, len(l.events))
	for _, e := range l.events {
		if e.Kind == "raw" && e.Seq <= through && len(e.Raw) > 0 {
			freed += int64(len(e.Raw))
			e.Raw = nil
		}
		keep = append(keep, e)
	}
	if freed == 0 {
		return 0, nil
	}
	path := l.path
	if path == "" {
		path = l.f.Name()
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".history-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(f.Name())
	adopted := false
	defer func() {
		if !adopted {
			f.Close()
		}
	}()
	enc := json.NewEncoder(f)
	for _, e := range keep {
		if err = enc.Encode(e); err != nil {
			return 0, err
		}
	}
	if err = f.Sync(); err != nil {
		return 0, err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return 0, err
	}
	old := l.f
	l.f = f
	l.path = path
	l.events = keep
	adopted = true
	old.Close()
	// Same bargain as a compaction: the rename is visible but its durability is
	// not established until the directory is synced, and a writer must not
	// acknowledge anything on a generation that may not survive a crash.
	if err := core.SyncDir(filepath.Dir(path)); err != nil {
		return freed, &CompactionCommitError{err}
	}
	return freed, nil
}

// RawBytes is how much of this journal is the verbatim vendor stream.
func (l *Log) RawBytes() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	var n int64
	for _, e := range l.events {
		if e.Kind == "raw" {
			n += int64(len(e.Raw))
		}
	}
	return n
}
