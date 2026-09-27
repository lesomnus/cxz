// Package journal is the supervisor's durable source of truth. SQLite is a projection.
package journal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Log struct {
	mu     sync.Mutex
	f      *os.File
	path   string
	events []core.Event
}

func Open(path string) (*Log, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	l := &Log{f: f, path: path}
	if e = core.SyncDir(filepath.Dir(path)); e != nil {
		f.Close()
		return nil, e
	}
	r := bufio.NewReader(f)
	var offset int64
	for {
		b, err := r.ReadBytes('\n')
		if err == io.EOF {
			if len(b) > 0 { // Preserve an incomplete final record for diagnostics before truncating it.
				if e = os.WriteFile(path+fmt.Sprintf(".partial-%d", time.Now().UnixNano()), b, 0600); e != nil {
					f.Close()
					return nil, e
				}
				if e = f.Truncate(offset); e != nil {
					f.Close()
					return nil, e
				}
				if e = f.Sync(); e != nil {
					f.Close()
					return nil, e
				}
			}
			break
		}
		if err != nil {
			f.Close()
			return nil, err
		}
		batch, err := decode(b, lastSeq(l.events))
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("corrupt committed record at %d: %w", offset, err)
		}
		l.events = append(l.events, batch...)
		offset += int64(len(b))
	}
	_, e = f.Seek(0, io.SeekEnd)
	if e != nil {
		f.Close()
		return nil, e
	}
	return l, nil
}
func (l *Log) Append(v core.Event) (core.Event, error) {
	batch, e := l.AppendBatch([]core.Event{v})
	if e != nil {
		return v, e
	}
	return batch[0], nil
}

// AppendBatch commits raw vendor input and all its derived events as one JSONL
// record. A torn record exposes none of the batch, never a partially projected input.
func (l *Log) AppendBatch(batch []core.Event) ([]core.Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(batch) == 0 {
		return nil, fmt.Errorf("empty journal batch")
	}
	batch = append([]core.Event(nil), batch...)
	for i := range batch {
		batch[i].Seq = lastSeq(l.events) + uint64(i) + 1
		if batch[i].TimeMS == 0 {
			batch[i].TimeMS = time.Now().UnixMilli()
		}
	}
	b, e := json.Marshal(batch)
	if e != nil {
		return nil, e
	}
	b = append(b, '\n')
	if _, e = l.f.Write(b); e != nil {
		return nil, e
	}
	if e = l.f.Sync(); e != nil {
		return nil, e
	}
	l.events = append(l.events, batch...)
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
func (l *Log) Close() error      { return l.f.Close() }

// Read observes a live journal without repairing its uncommitted tail.
func Read(path string) ([]core.Event, error) {
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	r := bufio.NewReader(f)
	var events []core.Event
	for {
		b, e := r.ReadBytes('\n')
		if e == io.EOF {
			return events, nil
		}
		if e != nil {
			return nil, e
		}
		batch, e := decode(b, lastSeq(events))
		if e != nil {
			return nil, e
		}
		events = append(events, batch...)
	}
}

func decode(b []byte, after uint64) ([]core.Event, error) {
	var batch []core.Event
	if len(b) > 0 && b[0] == '[' {
		if e := json.Unmarshal(b, &batch); e != nil {
			return nil, e
		}
	} else {
		var v core.Event
		if e := json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		batch = []core.Event{v}
	}
	if len(batch) == 0 {
		return nil, fmt.Errorf("empty committed batch")
	}
	if after == 0 && len(batch) == 1 && batch[0].Kind == core.HistoryCheckpointKind {
		var c core.HistoryCheckpoint
		if json.Unmarshal(batch[0].Payload, &c) != nil || c.Version != 1 || batch[0].Seq == 0 || c.Snapshot.LastSeq != batch[0].Seq {
			return nil, fmt.Errorf("invalid history checkpoint")
		}
		return batch, nil
	}
	for i, v := range batch {
		if v.Kind == core.HistoryCheckpointKind {
			return nil, fmt.Errorf("checkpoint must be the first record")
		}
		if v.Seq != after+uint64(i)+1 {
			return nil, fmt.Errorf("noncontiguous journal sequence")
		}
	}
	return batch, nil
}

func lastSeq(events []core.Event) uint64 {
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].Seq
}
