package supervisor

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
)

func assistantEvents(s *Supervisor) []core.Event {
	var out []core.Event
	for _, e := range s.log.All() {
		if e.Kind == "assistant" {
			out = append(out, e)
		} else if e.Response != nil {
			panic("response metadata on non-assistant event")
		}
	}
	return out
}

func TestClaudeResponseSnapshot(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.appliedModel, s.appliedEffort = "applied", "high"
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "one", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	// A subsequent settings report must not relabel this turn.
	s.appliedModel, s.appliedEffort = "next", "low"
	s.consume([]byte(`{"type":"assistant","message":{"model":"reported","content":[{"type":"text","text":"first"},{"type":"text","text":"second"}]}}`))
	want := core.ResponseMetadata{Model: "reported", Effort: "high", ModelSource: "response", EffortSource: "settings"}
	for _, e := range assistantEvents(s) {
		if e.Response == nil || *e.Response != want || len(e.Payload) != 0 {
			t.Fatalf("snapshot/raw payload changed: %+v", e)
		}
	}
	s.consume([]byte(`{"type":"result","subtype":"success"}`))
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "two", Text: "again"}); err != nil {
		t.Fatal(err)
	}
	s.consume([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"third"}]}}`))
	events := assistantEvents(s)
	if len(events) != 3 || *events[0].Response != want || events[2].Response.Model != "next" || events[2].Response.Effort != "low" {
		t.Fatalf("history relabeled: %+v", events)
	}
	// Check the actual durable journal, including compatibility with an old event.
	path := filepath.Join(t.TempDir(), "events")
	l, err := journal.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = l.AppendBatch(append(events, core.Event{Kind: "assistant", Text: "legacy"}))
	if err != nil {
		t.Fatal(err)
	}
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	l, err = journal.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	reloaded := l.All()
	if !reflect.DeepEqual(reloaded[0].Response, events[0].Response) || reloaded[3].Response != nil {
		t.Fatalf("snapshot not durable or legacy changed: %+v", reloaded)
	}
}

func TestClaudeSnapshotUnknownEffort(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.session.Model, s.effort = "requested", "high"
	if got := s.claudeResponseContext(); got.ModelSource != "requested" || got.EffortSource != "requested" {
		t.Fatal(got)
	}
	s.appliedModel = "confirmed"
	if got := s.claudeResponseContext(); got.Effort != "" || got.EffortSource != "" || got.ModelSource != "settings" {
		t.Fatal("invented applied effort", got)
	}
}

func TestQueuedClaudeInputCapturesSettingsAtDelivery(t *testing.T) {
	s, input := displaySupervisor(t, "claude")
	s.snap.State = "working"
	s.responseContext = core.ResponseMetadata{Model: "previous", Effort: "high"}
	s.appliedModel, s.appliedEffort = "at-queue", "high"
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "queued", Text: "wait"}); err != nil {
		t.Fatal(err)
	}
	if input.Len() != 0 || s.responseContext.Model != "previous" {
		t.Fatal("queued input changed the running turn")
	}
	s.appliedModel, s.appliedEffort = "at-delivery", "low"
	s.consume([]byte(`{"type":"result","subtype":"success"}`))
	s.consume([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"queued reply"}]}}`))
	e := assistantEvents(s)[0]
	if e.Response.Model != "at-delivery" || e.Response.Effort != "low" {
		t.Fatal("snapshot captured before delivery", e.Response)
	}
}

func TestCodexResponseSnapshotMatchesTurnRequest(t *testing.T) {
	s, input := displaySupervisor(t, "codex")
	s.modelOptions = []agentview.ModelOption{{ID: "catalog-default", Default: true, DefaultEffort: "medium"}}
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "one", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Params struct{ Model, Effort string }
	}
	if err := json.Unmarshal(bytes.TrimSpace(input.Bytes()), &wire); err != nil {
		t.Fatal(err)
	}
	s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"turn-1"}}}`))
	s.session.Model, s.effort = "next", "high"
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "steer", Text: "addendum"}); err != nil {
		t.Fatal(err)
	}
	s.consume([]byte(`{"id":"steer","result":{}}`))
	native := []byte(`{"method":"item/completed","params":{"turnId":"turn-1","item":{"type":"agentMessage","id":"answer","text":"reply"}}}`)
	s.consume(native)
	e := assistantEvents(s)[0]
	want := core.ResponseMetadata{Model: wire.Params.Model, Effort: wire.Params.Effort, ModelSource: "requested", EffortSource: "requested", TurnID: "turn-1"}
	if want.Model != "catalog-default" || want.Effort != "medium" || e.Response == nil || *e.Response != want || !bytes.Contains(e.Payload, []byte(`"turnId":"turn-1"`)) {
		t.Fatalf("wrong request snapshot or changed payload: %+v / %+v", e.Response, wire.Params)
	}
	s.consume([]byte(`{"method":"turn/completed","params":{"turn":{"id":"turn-1","status":"completed"}}}`))
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "two", Text: "next"}); err != nil {
		t.Fatal(err)
	}
	s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"turn-2"}}}`))
	s.consume(native) // Late notification must not receive the new turn's settings.
	s.consume([]byte(`{"method":"item/completed","params":{"turnId":"turn-2","item":{"type":"agentMessage","id":"answer-2","text":"new reply"}}}`))
	events := assistantEvents(s)
	if *events[0].Response != want || events[1].Response.Model != "" || events[2].Response.Model != "next" || events[2].Response.Effort != "high" {
		t.Fatalf("turn snapshots crossed: %+v / %+v / %+v", events[0].Response, events[1].Response, events[2].Response)
	}
	// An unsolicited turn has no matching request snapshot.
	s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"unsolicited"}}}`))
	s.consume([]byte(`{"method":"item/completed","params":{"turnId":"unsolicited","item":{"type":"agentMessage","id":"unknown","text":"unknown settings"}}}`))
	unknown := assistantEvents(s)[3].Response
	if unknown.Model != "" || unknown.Effort != "" || unknown.TurnID != "unsolicited" {
		t.Fatal("borrowed another turn's settings", unknown)
	}
}
