package journal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func TestCursorCommittedSuffixAndReplacement(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "events")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.Append(core.Event{Kind: "state", Text: "idle"}); err != nil {
		t.Fatal(err)
	}
	l.Close()
	first, cursor, reset, err := ReadSince(ctx, path, Cursor{})
	if err != nil || !reset || len(first) != 1 || cursor.Seq != 1 {
		t.Fatal(first, cursor, reset, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	batch, _ := json.Marshal([]core.Event{{Seq: 2, Kind: "raw"}, {Seq: 3, Kind: "assistant", Text: "tail"}})
	if _, err = f.Write(batch[:len(batch)/2]); err != nil {
		t.Fatal(err)
	}
	events, partial, reset, err := ReadSince(ctx, path, cursor)
	if err != nil || reset || len(events) != 0 || partial.Offset != cursor.Offset || partial.Seq != 1 {
		t.Fatal(events, partial, reset, err)
	}
	if _, err = f.Write(append(batch[len(batch)/2:], '\n')); err != nil {
		t.Fatal(err)
	}
	events, tail, reset, err := ReadSince(ctx, path, partial)
	if err != nil || reset || len(events) != 2 || events[0].Seq != 2 || tail.Seq != 3 {
		t.Fatal(events, tail, reset, err)
	}
	events, unchanged, reset, err := ReadSince(ctx, path, tail)
	if err != nil || reset || len(events) != 0 || unchanged.Offset != tail.Offset {
		t.Fatal(events, reset, err)
	}
	// A new inode must be read from the beginning even if it contains more data.
	replacement := path + ".new"
	l, err = Open(replacement)
	if err != nil {
		t.Fatal(err)
	}
	l.Append(core.Event{Kind: "input", Text: strings.Repeat("replacement", 100)})
	l.Close()
	if err = os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	events, next, reset, err := ReadSince(ctx, path, tail)
	if err != nil || !reset || next.Seq != 1 || len(events) != 1 || events[0].Kind != "input" {
		t.Fatal(events, reset, err)
	}
	if err = os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	events, next, reset, err = ReadSince(ctx, path, next)
	if err != nil || !reset || next.Seq != 0 || len(events) != 0 {
		t.Fatal(events, reset, err)
	}
}

func TestCursorRejectsCorruptionWithoutAdvancing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events")
	if err := os.WriteFile(path, []byte("{\"seq\":1}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, cursor, _, err := ReadSince(context.Background(), path, Cursor{})
	if err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString("{\"seq\":2}\n{\"seq\":9}\n")
	f.Close()
	events, next, _, err := ReadSince(context.Background(), path, cursor)
	if err == nil || len(events) != 0 || next.Seq != cursor.Seq || next.Offset != cursor.Offset {
		t.Fatal(events, next, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err = ReadSince(ctx, path, cursor); err != context.Canceled {
		t.Fatal(err)
	}
}

func BenchmarkJournalRead(b *testing.B) {
	for _, count := range []int{1000, 100000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "events")
			f, err := os.Create(path)
			if err != nil {
				b.Fatal(err)
			}
			enc := json.NewEncoder(f)
			for i := 1; i <= count; i++ {
				if err = enc.Encode(core.Event{Seq: uint64(i), Kind: "assistant", Text: strings.Repeat("x", 256)}); err != nil {
					b.Fatal(err)
				}
			}
			f.Close()
			_, cursor, _, err := ReadSince(context.Background(), path, Cursor{})
			if err != nil {
				b.Fatal(err)
			}
			b.Run("full", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := Read(path); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("unchanged", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, _, _, err := ReadSince(context.Background(), path, cursor); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
