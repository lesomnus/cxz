package journal

import (
	"bytes"
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

// Shedding empties a vendor payload where it was rather than removing the
// event: a sequence that is missing and one that was never written look the
// same, and everything else in a journal finds its place by sequence.
func TestShedEmptiesVendorPayloadsInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	fat := bytes.Repeat([]byte("x"), 4096)
	for i := 0; i < 4; i++ {
		if _, err = log.Append(core.Event{SessionID: "s", Kind: "input", Text: "said"}); err != nil {
			t.Fatal(err)
		}
		if _, err = log.Append(core.Event{SessionID: "s", Kind: "raw", Raw: fat}); err != nil {
			t.Fatal(err)
		}
	}
	before := log.All()
	if log.RawBytes() != int64(4*len(fat)) {
		t.Fatal("fixture", log.RawBytes())
	}
	size, err := log.Size()
	if err != nil {
		t.Fatal(err)
	}
	// Through the second raw event.
	freed, err := log.Shed(before[3].Seq)
	if err != nil || freed != int64(2*len(fat)) {
		t.Fatal(freed, err)
	}
	after := log.All()
	if len(after) != len(before) {
		t.Fatal("an event was removed", len(after), len(before))
	}
	for i := range after {
		if after[i].Seq != before[i].Seq || after[i].Kind != before[i].Kind || after[i].Text != before[i].Text {
			t.Fatal("an event changed identity", i, after[i])
		}
	}
	if len(after[1].Raw) != 0 || len(after[3].Raw) != 0 {
		t.Fatal("a shed payload survived")
	}
	if len(after[5].Raw) == 0 || len(after[7].Raw) == 0 {
		t.Fatal("a payload above the boundary was shed")
	}
	if now, _ := log.Size(); now >= size {
		t.Fatal("the file did not shrink", size, now)
	}
	// What is on disk is what is in memory, and shedding again is nothing.
	reread, err := Read(path)
	if err != nil || len(reread) != len(after) {
		t.Fatal(reread, err)
	}
	for i := range reread {
		if len(reread[i].Raw) != len(after[i].Raw) {
			t.Fatal("disk and memory disagree", i)
		}
	}
	if freed, err = log.Shed(before[3].Seq); err != nil || freed != 0 {
		t.Fatal("shedding twice freed something", freed, err)
	}
	// A live reader of the replaced file continues from where it was.
	if _, err = log.Append(core.Event{SessionID: "s", Kind: "input", Text: "after"}); err != nil {
		t.Fatal(err)
	}
	if tail := log.After(after[len(after)-1].Seq, 10); len(tail) != 1 || tail[0].Text != "after" {
		t.Fatal("the journal did not continue", tail)
	}
}
