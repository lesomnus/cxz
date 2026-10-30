package supervisor

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
)

func backgroundReply(t *testing.T, s *Supervisor, result any) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"id": json.RawMessage(s.codex.background.id), "result": result})
	if err != nil {
		t.Fatal(err)
	}
	s.consume(raw)
}

func backgroundEvents(t *testing.T, s *Supervisor) []json.RawMessage {
	t.Helper()
	var events []json.RawMessage
	for _, e := range s.log.All() {
		if e.Kind == "background" {
			events = append(events, e.Payload)
		}
	}
	return events
}

func TestCodexBackgroundPaginatedLifecycle(t *testing.T) {
	s, input := displaySupervisor(t, "codex")
	c := s.codex
	c.readBackground(time.Now(), true)
	if !bytes.Contains(input.Bytes(), []byte(`"method":"thread/backgroundTerminals/list"`)) || !bytes.Contains(input.Bytes(), []byte(`"threadId":"thread"`)) {
		t.Fatal(input.String())
	}
	before := input.Len()
	c.readBackground(time.Now(), true)
	if input.Len() != before {
		t.Fatal("overlapping list requests")
	}
	backgroundReply(t, s, map[string]any{"data": []any{map[string]any{"processId": "42", "itemId": "tool-a", "command": "sleep 60"}}, "nextCursor": "page-2"})
	if len(backgroundEvents(t, s)) != 0 {
		t.Fatal("partial list published")
	}
	if !bytes.Contains(input.Bytes(), []byte(`"cursor":"page-2"`)) {
		t.Fatal("no second page", input.String())
	}
	backgroundReply(t, s, map[string]any{"data": []any{map[string]any{"processId": "43", "itemId": "tool-b", "command": "server"}, map[string]any{"processId": "42", "itemId": "tool-a", "command": "sleep 60"}}})
	var state agentview.BackgroundState
	for _, raw := range backgroundEvents(t, s) {
		state.Apply(raw)
	}
	if len(state.Tasks) != 2 || !state.Snapshot || state.Unavailable || !state.Tasks["42"].Active || state.Tasks["42"].ToolID != "tool-a" {
		t.Fatalf("%+v", state)
	}
	// Same identities/commands with different CPU/RSS samples do not update UI.
	c.readBackground(time.Now(), true)
	backgroundReply(t, s, map[string]any{"data": []any{map[string]any{"processId": "43", "itemId": "tool-b", "command": "server", "cpuPercent": 4}, map[string]any{"processId": "42", "itemId": "tool-a", "command": "sleep 60", "rssKb": 900}}})
	if len(backgroundEvents(t, s)) != 1 {
		t.Fatal("unchanged list published")
	}
	s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"turn"}}}`))
	s.consume([]byte(`{"method":"turn/completed","params":{"turn":{"id":"turn","status":"completed"}}}`))
	backgroundReply(t, s, map[string]any{"data": []any{}})
	events := backgroundEvents(t, s)
	state.Apply(events[len(events)-1])
	if !state.Snapshot || state.Unavailable || state.Tasks["42"].Active || state.Tasks["43"].Active || state.Tasks["42"].Status != "inactive" {
		t.Fatalf("%+v", state)
	}
	if !bytes.Contains(events[len(events)-1], []byte(`"tasks":[]`)) {
		t.Fatal("empty list is not authoritative", string(events[len(events)-1]))
	}
	checkpoint, err := makeHistoryCheckpoint(s.log.All())
	if err != nil {
		t.Fatal(err)
	}
	var retained agentview.BackgroundState
	for _, event := range resumeEvents([]core.Event{checkpoint}) {
		if event.Kind == "background" {
			retained.Apply(event.Payload)
		}
	}
	if !retained.Snapshot || retained.Unavailable {
		t.Fatal("background snapshot lost at history checkpoint")
	}
}

func TestCodexBackgroundFailureIsolation(t *testing.T) {
	for _, code := range []int{-32601, -32000} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			s, input := displaySupervisor(t, "codex")
			s.snap.State = "working"
			s.codex.turn = "active"
			s.codex.readBackground(time.Now(), true)
			raw, _ := json.Marshal(map[string]any{"id": json.RawMessage(s.codex.background.id), "error": map[string]any{"code": code, "message": "unavailable"}})
			s.consume(raw)
			if s.snap.State != "working" || s.codex.turn != "active" || !s.updateBackground.Unavailable {
				t.Fatal("telemetry changed conversation state")
			}
			for _, e := range s.log.All() {
				if e.Kind == "turn_end" || e.Kind == "diagnostic" {
					t.Fatal("telemetry failure became conversation error", e)
				}
			}
			before := input.Len()
			s.codex.readBackground(time.Now(), true)
			if input.Len() != before {
				t.Fatal("unsupported/error request flooded")
			}
			s.codex.readBackground(time.Now().Add(codexBackgroundRetry), false)
			if code == -32601 && input.Len() != before {
				t.Fatal("unsupported API retried")
			}
			if code != -32601 {
				backgroundReply(t, s, map[string]any{"data": []any{}})
				if s.updateBackground.Unavailable || !s.updateBackground.Snapshot {
					t.Fatal("did not recover")
				}
			}
		})
	}
}

func TestCodexBackgroundTimeoutRejectsLatePages(t *testing.T) {
	s, input := displaySupervisor(t, "codex")
	c := s.codex
	now := time.Now()
	c.readBackground(now, true)
	old := c.background.id
	c.readBackground(now.Add(codexBackgroundTimeout), false)
	if !s.updateBackground.Unavailable {
		t.Fatal("timed-out telemetry stayed available")
	}
	c.readBackground(now.Add(codexBackgroundTimeout+codexBackgroundRetry), false)
	current := c.background.id
	if current == "" || current == old {
		t.Fatal("request ID reused")
	}
	raw, _ := json.Marshal(map[string]any{"id": json.RawMessage(old), "result": map[string]any{"data": []any{}}})
	s.consume(raw)
	if !s.updateBackground.Unavailable || c.background.id != current {
		t.Fatal("late result overwrote current request")
	}
	backgroundReply(t, s, map[string]any{"data": []any{}})
	if s.updateBackground.Unavailable {
		t.Fatal("new list did not recover")
	}
	before := input.Len()
	c.readBackground(time.Now(), false)
	if input.Len() != before {
		t.Fatal("periodic refresh ignored interval")
	}
	s.stopping = true
	c.readBackground(time.Now().Add(time.Minute), true)
	if input.Len() != before {
		t.Fatal("polled stopping session")
	}
}

func TestCodexBackgroundInvalidPaginationNeverPublishesPartialState(t *testing.T) {
	for _, result := range []any{map[string]any{}, map[string]any{"data": []any{map[string]any{"command": "missing ID"}}}, map[string]any{"data": []any{}, "nextCursor": "again"}} {
		s, _ := displaySupervisor(t, "codex")
		s.codex.readBackground(time.Now(), true)
		backgroundReply(t, s, map[string]any{"data": []any{map[string]any{"processId": "42"}}, "nextCursor": "again"})
		backgroundReply(t, s, result)
		if !s.updateBackground.Unavailable || len(s.updateBackground.Tasks) != 0 {
			t.Fatal("partial or malformed list became count", s.updateBackground)
		}
	}
}
