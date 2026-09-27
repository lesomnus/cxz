package journal

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/core"
	"os"
	"path/filepath"
	"testing"
)

func TestCompactionPreservesSequencesAndLiveCursor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if _, err = l.Append(core.Event{Kind: "assistant", Text: "payload"}); err != nil {
			t.Fatal(err)
		}
	}
	_, cursor, _, err := ReadSince(context.Background(), path, Cursor{})
	if err != nil {
		t.Fatal(err)
	}
	for _, floor := range []uint64{10, 15} {
		payload, _ := json.Marshal(core.HistoryCheckpoint{Version: 1, Snapshot: core.Snapshot{LastSeq: floor, State: "idle", VendorID: "vendor"}})
		if err = l.Compact(core.Event{Kind: core.HistoryCheckpointKind, Seq: floor, Payload: payload}); err != nil {
			t.Fatal(err)
		}
		events, next, reset, err := ReadSince(context.Background(), path, cursor)
		if err != nil || !reset || events[0].Seq != floor || next.Seq != 20 {
			t.Fatal(events, next, reset, err)
		}
		cursor = next
		if got := l.After(floor-5, 2); len(got) != 2 || got[0].Seq != floor || got[1].Seq != floor+1 {
			t.Fatal(got)
		}
	}
	l.Close()
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString(`{"seq":21`)
	f.Close()
	l, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	e, err := l.Append(core.Event{Kind: "state", Text: "idle"})
	if err != nil || e.Seq != 21 {
		t.Fatal(e, err)
	}
	if all, err := Read(path); err != nil || len(all) != 7 || all[0].Seq != 15 || all[6].Seq != 21 {
		t.Fatal(all, err)
	}
}
