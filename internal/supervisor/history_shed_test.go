package supervisor

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"github.com/lesomnus/cxz/internal/journal"
)

func shedFixture(t *testing.T) (*Supervisor, *journal.Log, string) {
	t.Helper()
	root := t.TempDir()
	log, err := journal.Open(filepath.Join(root, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Supervisor{
		session: core.Session{ID: "session", Kind: "codex"}, log: log,
		snap: core.Snapshot{RunID: "run", State: "idle"}, receipts: map[string]record{},
	}
	return s, log, root
}

// A turn of conversation with a fat vendor stream beside it, which is the shape
// a real journal has: a few hundred bytes of what was said and a megabyte of
// how it was transported.
func shedTurns(s *Supervisor, turns int, rawPerTurn int) {
	for i := 0; i < turns; i++ {
		s.event("input", "prompt", "", nil, nil)
		s.event("state", "working", "", nil, nil)
		s.event("raw", "", "", nil, []byte(`{"stream":"`+strings.Repeat("x", rawPerTurn)+`"}`))
		s.event("assistant", "the answer", "", nil, nil)
		s.event("turn_end", "completed", "", nil, nil)
		s.event("state", "idle", "", nil, nil)
	}
}

func conversationText(events []core.Event) []string {
	var out []string
	for _, e := range events {
		if e.Kind == "input" || e.Kind == "assistant" {
			out = append(out, e.Text)
		}
	}
	return out
}

// The point of the whole thing: a journal that is mostly vendor stream loses the
// vendor stream, not the conversation. Before this, the disk budget was spent on
// telemetry and the trim that followed removed turns to get under it.
func TestTelemetryIsShedBeforeConversationIsTrimmed(t *testing.T) {
	s, log, root := shedFixture(t)
	shedTurns(s, 12, 200<<10)
	before := len(conversationText(log.All()))
	if before != 24 {
		t.Fatal("fixture", before)
	}
	// A budget the vendor stream is far over and the conversation is nowhere
	// near: 2.4 MiB of stream, a few hundred bytes of what was said.
	if err := historypolicy.Save(root, historypolicy.Policy{MaxMiB: 10, RawMiB: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.retainHistory(root, func() error { t.Error("a checkpoint was authorized for a telemetry shed"); return nil }); err != nil {
		t.Fatal(err)
	}
	events := log.All()
	if got := len(conversationText(events)); got != before {
		t.Fatalf("conversation was taken: %d of %d", got, before)
	}
	if events[0].Kind == core.HistoryCheckpointKind {
		t.Fatal("the journal was trimmed instead of shed")
	}
	// Under its budget now, and what is left is the most recent stream.
	raw := log.RawBytes()
	if raw > 1<<20 {
		t.Fatal("still over the telemetry budget", raw)
	}
	if raw == 0 {
		t.Fatal("every vendor payload was shed, including the recent ones")
	}
	// The stub stays where the payload was, so a reader can tell that a turn
	// produced a stream from a turn that did not.
	stubs, kept := 0, 0
	for _, e := range events {
		if e.Kind != "raw" {
			continue
		}
		if len(e.Raw) == 0 {
			stubs++
		} else {
			kept++
		}
	}
	if stubs == 0 || kept == 0 {
		t.Fatal("expected shed stubs and kept payloads", stubs, kept)
	}
	// And it says so in the journal rather than leaving a reader to wonder.
	marker := core.Event{}
	for _, e := range events {
		if e.Kind == core.HistoryShedKind {
			marker = e
		}
	}
	if marker.Seq == 0 || core.HistoryFloor(marker.Kind, marker.Payload) != 0 {
		t.Fatal("no shed marker, or one that reads as a floor", marker)
	}
	var boundary core.HistoryBoundary
	if err := json.Unmarshal(marker.Payload, &boundary); err != nil || boundary.Through == 0 {
		t.Fatal("the marker does not say how far it went", err, boundary)
	}
}

// The most recent turn keeps its stream: that is the one a person opens the raw
// view on, and the one background work is reconstructed from.
func TestTheLastTurnKeepsItsStream(t *testing.T) {
	s, log, root := shedFixture(t)
	shedTurns(s, 2, 400<<10)
	if err := historypolicy.Save(root, historypolicy.Policy{RawMiB: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.retainHistory(root, nil); err != nil {
		t.Fatal(err)
	}
	events := log.All()
	last := core.Event{}
	for _, e := range events {
		if e.Kind == "raw" {
			last = e
		}
	}
	if len(last.Raw) == 0 {
		t.Fatal("the latest vendor payload was shed")
	}
}

// Turning retention off turns both budgets off: one switch, not two.
func TestDisabledRetentionShedsNothing(t *testing.T) {
	s, log, root := shedFixture(t)
	shedTurns(s, 12, 200<<10)
	before := log.RawBytes()
	if err := historypolicy.Save(root, historypolicy.Policy{Disabled: true, RawMiB: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.retainHistory(root, nil); err != nil {
		t.Fatal(err)
	}
	if log.RawBytes() != before {
		t.Fatal("a disabled policy shed anyway")
	}
}

// Nothing is touched while a turn is in flight.
func TestNothingIsShedMidTurn(t *testing.T) {
	s, log, root := shedFixture(t)
	shedTurns(s, 12, 200<<10)
	s.event("input", "the next question", "", nil, nil)
	s.snap.State = "working"
	before := log.RawBytes()
	if err := historypolicy.Save(root, historypolicy.Policy{RawMiB: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.retainHistory(root, nil); err != nil {
		t.Fatal(err)
	}
	if log.RawBytes() != before {
		t.Fatal("shed while a turn was in flight")
	}
}
