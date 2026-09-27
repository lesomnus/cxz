package journal

import (
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"os"
	"path/filepath"
)

func (l *Log) Size() (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	i, err := l.f.Stat()
	if err != nil {
		return 0, err
	}
	return i.Size(), nil
}

// Compact replaces checkpoint and suffix as one durable file. The supervisor
// serializes this with event production. Readers with the old inode can finish;
// their next ReadSince detects the replacement and rebuilds its projection.
func (l *Log) Compact(checkpoint core.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if checkpoint.Kind != core.HistoryCheckpointKind || checkpoint.Seq == 0 {
		return fmt.Errorf("invalid checkpoint")
	}
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	if _, err = decode(encoded, 0); err != nil {
		return err
	}
	if len(l.events) == 0 || checkpoint.Seq < l.events[0].Seq || checkpoint.Seq > lastSeq(l.events) {
		return fmt.Errorf("checkpoint beyond journal")
	}
	keep := []core.Event{checkpoint}
	for _, e := range l.events {
		if e.Seq > checkpoint.Seq {
			keep = append(keep, e)
		}
	}
	path := l.path
	if path == "" {
		path = l.f.Name()
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".history-*")
	if err != nil {
		return err
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
			return err
		}
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	old := l.f
	l.f = f
	l.path = path
	l.events = keep
	adopted = true
	old.Close()
	if err := core.SyncDir(filepath.Dir(path)); err != nil {
		return &CompactionCommitError{err}
	}
	return nil
}

// CompactionCommitError means rename succeeded but its durability is uncertain.
// Writers must stop rather than acknowledge commands on an uncertain generation.
type CompactionCommitError struct{ Err error }

func (e *CompactionCommitError) Error() string {
	return "history checkpoint durability: " + e.Err.Error()
}
func (e *CompactionCommitError) Unwrap() error { return e.Err }
