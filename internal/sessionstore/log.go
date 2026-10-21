package sessionstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

// Log is a session's conversation as its supervisor holds it: every event in
// memory, and the store those events are written to.
//
// It has the shape `journal.Log` had, because a session reads its own
// conversation through this for retention, fork and the snapshot it replays at
// startup. What stage 5 changes is where the bytes land, not how a session uses
// them -- so the shape stays and the file underneath is the store.
// See cxz#278.
type Log struct {
	store *Store
	mu    sync.Mutex
	// events is the whole conversation, as the journal kept it. A session has
	// always held its own history in memory; reading it from a table rather
	// than a file does not change that.
	events []core.Event
	// raw is the bytes of verbatim vendor payload still stored, which is what
	// the shedding decision is made against.
	raw int64
}

// OpenLog opens a session's store for writing and reads what it already holds.
//
// A journal beside it is converted first, once: a session recorded before the
// store existed comes back that way, and after this nothing appends to that
// file again. Convert is re-runnable, so a session that has already been
// converted pays a single query for the privilege.
func OpenLog(ctx context.Context, dir, session string) (*Log, error) {
	if _, err := Convert(ctx, dir, session); err != nil {
		return nil, err
	}
	store, err := Open(ctx, Path(dir), session, true)
	if err != nil {
		return nil, err
	}
	events, err := store.Events(ctx)
	if err != nil {
		store.Close()
		return nil, err
	}
	l := &Log{store: store, events: events}
	for _, e := range events {
		l.raw += int64(len(e.Raw))
	}
	return l, nil
}

// Store is the file this log writes, for the recorder that keeps the session's
// snapshot beside its events.
func (l *Log) Store() *Store { return l.store }

func (l *Log) Close() error { return l.store.Close() }

// Append and AppendBatch number the events and commit them, exactly as the
// journal did: a batch is one transaction, so a reader sees all of an input and
// its derived events or none of them.
func (l *Log) Append(v core.Event) (core.Event, error) {
	batch, err := l.AppendBatch([]core.Event{v})
	if err != nil {
		return v, err
	}
	return batch[0], nil
}

func (l *Log) AppendBatch(batch []core.Event) ([]core.Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(batch) == 0 {
		return nil, fmt.Errorf("empty conversation batch")
	}
	batch = append([]core.Event(nil), batch...)
	for i := range batch {
		batch[i].Seq = lastSeq(l.events) + uint64(i) + 1
		if batch[i].TimeMS == 0 {
			batch[i].TimeMS = time.Now().UnixMilli()
		}
	}
	if err := l.store.Append(context.Background(), batch); err != nil {
		return nil, err
	}
	l.events = append(l.events, batch...)
	for _, e := range batch {
		l.raw += int64(len(e.Raw))
	}
	return batch, nil
}

func (l *Log) After(seq uint64, limit int) []core.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	start := sort.Search(len(l.events), func(i int) bool { return l.events[i].Seq > seq })
	end := start + min(len(l.events)-start, max(0, limit))
	return append([]core.Event(nil), l.events[start:end]...)
}

func (l *Log) All() []core.Event { return l.After(0, int(^uint(0)>>1)) }

// Size is how much conversation is stored, which is what the retention policy
// bounds and all a trim can move. The file underneath is larger, and so are the
// pages in use -- see Store.Bytes for why neither is the measure.
func (l *Log) Size() (int64, error) {
	return l.store.Bytes(context.Background())
}

// RawBytes is the verbatim vendor payload still stored.
func (l *Log) RawBytes() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.raw
}

// Shed empties the verbatim payload of every raw event at or below through and
// answers with the bytes that are no longer stored.
//
// It empties rather than deletes, for the reason the journal did: an event that
// is gone and an event that was never recorded look the same to a reader, and
// the sequence it occupied would become a hole in the numbering everything else
// finds its place by. A raw event with nothing in it still says a turn produced
// one.
func (l *Log) Shed(through uint64) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var freed int64
	var shed []core.Event
	for i := range l.events {
		e := &l.events[i]
		if e.Kind != "raw" || e.Seq > through || len(e.Raw) == 0 {
			continue
		}
		freed += int64(len(e.Raw))
		e.Raw = nil
		shed = append(shed, *e)
	}
	if freed == 0 {
		return 0, nil
	}
	if err := l.store.Replace(context.Background(), shed); err != nil {
		return 0, err
	}
	l.raw -= freed
	return freed, nil
}

// Compact replaces everything up to and including the checkpoint with the
// checkpoint itself, and keeps the suffix.
func (l *Log) Compact(checkpoint core.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if checkpoint.Kind != core.HistoryCheckpointKind || checkpoint.Seq == 0 {
		return fmt.Errorf("invalid checkpoint")
	}
	if _, err := json.Marshal(checkpoint); err != nil {
		return err
	}
	if len(l.events) == 0 || checkpoint.Seq < l.events[0].Seq || checkpoint.Seq > lastSeq(l.events) {
		return fmt.Errorf("checkpoint beyond conversation")
	}
	if err := l.store.Trim(context.Background(), checkpoint); err != nil {
		return err
	}
	keep := []core.Event{checkpoint}
	var raw int64
	for _, e := range l.events {
		if e.Seq > checkpoint.Seq {
			keep = append(keep, e)
			raw += int64(len(e.Raw))
		}
	}
	l.events = keep
	l.raw = raw
	return nil
}

func lastSeq(events []core.Event) uint64 {
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].Seq
}

// Fence is how much of a session's store is on disk and when it last changed,
// for a caller that wants to know whether the conversation has moved without
// reading it.
//
// Both files, because in WAL mode a commit lands in the write-ahead log and the
// database itself only changes when that log is checkpointed. This is what the
// journal's size and mtime used to answer. See cxz#278.
func Fence(dir string) (size, mtimeNS int64, err error) {
	var found bool
	for _, path := range []string{Path(dir), Path(dir) + "-wal"} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return 0, 0, statErr
		}
		found = true
		size += info.Size()
		if ns := info.ModTime().UnixNano(); ns > mtimeNS {
			mtimeNS = ns
		}
	}
	if !found {
		return 0, 0, os.ErrNotExist
	}
	return size, mtimeNS, nil
}
