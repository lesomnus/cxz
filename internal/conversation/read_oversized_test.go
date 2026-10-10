package conversation

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

// raw appends a committed line the journal writer would not produce, which is
// the only way to put a record of a chosen size between two readable ones.
func (f *fixture) raw(t *testing.T, line string) {
	t.Helper()
	path := filepath.Join(core.Dir(f.store.Root, f.target.RuntimeID), "events.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = file.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) event(t *testing.T, seq uint64, kind, text string) string {
	t.Helper()
	b, err := json.Marshal(core.Event{
		SessionID: f.target.RuntimeID, Seq: seq, Kind: kind, Text: text,
		TimeMS: f.now.Add(-5 * time.Minute).UnixMilli(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A runaway record used to take the whole session with it: bufio.Scanner cannot
// resume past an over-long token, so every record read before it was discarded
// and the reply said nothing about the session at all. The records on both sides
// of it are committed and have to come back.
func TestOneUnreadableRecordDoesNotLoseTheRest(t *testing.T) {
	f := setup(t)
	f.store.recordLimit = 4 << 10
	f.add(t, "input", "before", nil)
	f.add(t, "assistant", "also before", nil)
	f.raw(t, f.event(t, 3, "input", strings.Repeat("x", 8<<10)))
	f.raw(t, f.event(t, 4, "input", "after"))

	r, err := f.store.Read(t.Context(), Query{Session: "seal"})
	if err != nil {
		t.Fatal(err)
	}
	var seqs []uint64
	for _, e := range r.Bodies {
		seqs = append(seqs, e.Seq)
	}
	if len(seqs) != 3 || seqs[0] != 1 || seqs[1] != 2 || seqs[2] != 4 {
		t.Fatal("records around the skipped one were lost:", seqs)
	}
	if len(r.UnreadableSeqs) != 1 || r.UnreadableSeqs[0] != 3 {
		t.Fatal("the skipped record was not named:", r.UnreadableSeqs)
	}
	// The size is the one number that says whether anything can be done about
	// it, and the limit is what it is being measured against.
	if !strings.Contains(r.Message, "seq 3") || !strings.Contains(r.Message, "record limit") {
		t.Fatal("the reply does not say what happened:", r.Message)
	}
	// It is not OversizedSeqs: that field's remedies do nothing here, so a
	// caller must not be sent to retry with output=file.
	if len(r.OversizedSeqs) != 0 {
		t.Fatal("reported as a budget overflow:", r.OversizedSeqs)
	}
	// The snapshot still reaches the last committed record, so a cursor taken
	// from this page does not stop at the hole.
	if r.SnapshotSeq != 4 {
		t.Fatal("snapshot stopped at the skipped record:", r.SnapshotSeq)
	}

	// Lookup reads the same journal and used to fail outright, leaving no way
	// to learn anything about the session at all.
	v, err := f.store.Lookup(t.Context(), "seal")
	if err != nil {
		t.Fatal(err)
	}
	if v.FirstSeq != 1 || v.SnapshotSeq != 4 || len(v.UnreadableSeqs) != 1 {
		t.Fatalf("%+v", v)
	}
}

// A record too large to hold is skipped before anything tries to parse it, so
// one that is also malformed cannot be reported by sequence. It is still
// located, by the last sequence read before it, rather than guessed at.
func TestAnUnidentifiableRecordIsLocatedNotGuessed(t *testing.T) {
	f := setup(t)
	f.store.recordLimit = 4 << 10
	f.add(t, "input", "before", nil)
	f.raw(t, strings.Repeat("x", 8<<10))
	f.raw(t, f.event(t, 2, "input", "after"))

	r, err := f.store.Read(t.Context(), Query{Session: "seal"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Bodies) != 2 {
		t.Fatal("a malformed oversized record stopped the read:", len(r.Bodies))
	}
	if len(r.UnreadableSeqs) != 0 {
		t.Fatal("a sequence was invented for a record that named none:", r.UnreadableSeqs)
	}
	if !strings.Contains(r.Message, "after seq 1") {
		t.Fatal("the reply does not locate it:", r.Message)
	}
}

// A record without its newline is the one a supervisor is still writing. It was
// never read, and it must not be reported as skipped either -- a reader that
// called the live tail a lost record would be wrong every time a session is
// busy.
func TestAnUnterminatedOversizedTailIsNotReported(t *testing.T) {
	f := setup(t)
	f.store.recordLimit = 4 << 10
	f.add(t, "input", "committed", nil)
	path := filepath.Join(core.Dir(f.store.Root, f.target.RuntimeID), "events.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString(strings.Repeat("y", 8<<10)); err != nil {
		t.Fatal(err)
	}
	file.Close()

	r, err := f.store.Read(t.Context(), Query{Session: "seal"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Bodies) != 1 || len(r.UnreadableSeqs) != 0 || r.Message != "" {
		t.Fatalf("an uncommitted tail was reported: %+v", r)
	}
}

// nextRecord is where the scanner's cliff was. Over the limit it has to consume
// the rest of the record, keep enough of its start to name it, and leave the
// reader positioned on the next one.
func TestNextRecordSkipsAndContinues(t *testing.T) {
	data := "one\n" + strings.Repeat("z", 100) + "\ntwo\n"
	r := bufio.NewReaderSize(strings.NewReader(data), 16)

	record, oversize, complete, err := nextRecord(r, 50, 8)
	if err != nil || oversize != 0 || !complete || string(record) != "one\n" {
		t.Fatal(string(record), oversize, complete, err)
	}
	record, oversize, complete, err = nextRecord(r, 50, 8)
	if err != nil || !complete || oversize != 101 || string(record) != "zzzzzzzz" {
		t.Fatal(string(record), oversize, complete, err)
	}
	// The reader is on the record after the skipped one, not inside it.
	record, oversize, complete, err = nextRecord(r, 50, 8)
	if err != nil || oversize != 0 || !complete || string(record) != "two\n" {
		t.Fatal(string(record), oversize, complete, err)
	}
	// End of file: no record, and nothing to carry on to.
	if record, oversize, complete, _ = nextRecord(r, 50, 8); len(record) != 0 || oversize != 0 || complete {
		t.Fatal(string(record), oversize, complete)
	}
}

// recordSeq reads a number out of a fragment, which is all a skipped record
// leaves. core.Event puts seq ahead of every field that can be large, so the
// first match is the record's own -- not one quoted inside its text.
func TestRecordSeqReadsTheFragment(t *testing.T) {
	for _, tc := range []struct {
		name string
		head string
		want uint64
	}{
		{"single event", `{"session_id":"a","run_id":"r","seq":7,"time_ms":1,"kind":"input","text":"tr`, 7},
		{"batch names its first", `[{"session_id":"a","run_id":"r","seq":11,"time_ms":1,"kind":"input","text":"x"},{"seq":12`, 11},
		{"text cannot shadow it", `{"session_id":"a","seq":9,"kind":"input","text":"\"seq\": 999 and more`, 9},
		{"no number to find", strings.Repeat("x", 64), 0},
		{"empty", "", 0},
	} {
		if got := recordSeq([]byte(tc.head)); got != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
