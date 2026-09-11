package journal

import (
	"github.com/lesomnus/cxz/internal/core"
	"os"
	"path/filepath"
	"testing"
)

func TestRecovery(t *testing.T) {
	p := filepath.Join(t.TempDir(), "events.jsonl")
	l, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	for range 3 {
		if _, e = l.Append(core.Event{Kind: "test"}); e != nil {
			t.Fatal(e)
		}
	}
	l.Close()
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString(`{"seq":4`)
	f.Close()
	events, e := Read(p)
	if e != nil || len(events) != 3 {
		t.Fatalf("read %v %v", events, e)
	}
	l, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	v, e := l.Append(core.Event{Kind: "recovered"})
	if e != nil || v.Seq != 4 {
		t.Fatalf("append %v %v", v, e)
	}
	backups, _ := filepath.Glob(p + ".partial-*")
	if len(backups) != 1 {
		t.Fatal("missing diagnostic backup")
	}
}
func TestCorruptionFailsClosed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "events")
	os.WriteFile(p, []byte("bad\n"), 0600)
	if l, e := Open(p); e == nil {
		l.Close()
		t.Fatal("accepted corrupt journal")
	}
}

func TestAtomicBatch(t *testing.T) {
	p := filepath.Join(t.TempDir(), "events")
	l, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	batch, e := l.AppendBatch([]core.Event{{Kind: "raw"}, {Kind: "assistant"}, {Kind: "state", Text: "idle"}})
	if e != nil || len(batch) != 3 {
		t.Fatal(e)
	}
	l.Close()
	data, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	for _, cut := range []int{1, len(data) / 2, len(data) - 1} {
		other := p + "-torn"
		os.WriteFile(other, data[:cut], 0600)
		events, e := Read(other)
		if e != nil || len(events) != 0 {
			t.Fatal("partial batch exposed", e)
		}
		l, e = Open(other)
		if e != nil {
			t.Fatal(e)
		}
		if len(l.All()) != 0 {
			t.Fatal("partial batch recovered")
		}
		l.Close()
	}
	l, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	if len(l.All()) != 3 {
		t.Fatal("lost committed batch")
	}
}
