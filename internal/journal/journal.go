// Package journal is the supervisor's durable source of truth. SQLite is a projection.
package journal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"io"
	"os"
	"sync"
	"time"
)

type Log struct {
	mu     sync.Mutex
	f      *os.File
	events []core.Event
}

func Open(path string) (*Log, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	l := &Log{f: f}
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
		var v core.Event
		if e = json.Unmarshal(b, &v); e != nil {
			f.Close()
			return nil, fmt.Errorf("corrupt committed record at %d: %w", offset, e)
		}
		if v.Seq != uint64(len(l.events)+1) {
			f.Close()
			return nil, fmt.Errorf("noncontiguous journal sequence")
		}
		l.events = append(l.events, v)
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
	l.mu.Lock()
	defer l.mu.Unlock()
	v.Seq = uint64(len(l.events) + 1)
	v.TimeMS = time.Now().UnixMilli()
	b, e := json.Marshal(v)
	if e != nil {
		return v, e
	}
	b = append(b, '\n')
	if _, e = l.f.Write(b); e != nil {
		return v, e
	}
	if e = l.f.Sync(); e != nil {
		return v, e
	}
	l.events = append(l.events, v)
	return v, nil
}
func (l *Log) After(seq uint64, limit int) []core.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	if seq >= uint64(len(l.events)) {
		return nil
	}
	end := min(len(l.events), int(seq)+limit)
	return append([]core.Event(nil), l.events[int(seq):end]...)
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
		var v core.Event
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		if v.Seq != uint64(len(events)+1) {
			return nil, fmt.Errorf("noncontiguous journal sequence")
		}
		events = append(events, v)
	}
}
