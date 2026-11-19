package supervisor

import (
	"encoding/json"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

// A subagent's work arrives on the session's own stream: its replies and its
// tool calls are recorded as the session's events, and nothing but
// `parent_tool_use_id` says otherwise. Measured on a real session, 46
// tool_call and 45 tool_result events from three subagents were
// indistinguishable from the session's own. See cxz#368.
func subagentAssistant(parent, kind, text string) []byte {
	message := map[string]any{
		"model": "claude-sonnet-5-5",
		"content": []map[string]any{
			{"type": "text", "text": text},
			{"type": "tool_use", "id": "toolu_" + kind, "name": "Bash", "input": map[string]any{"command": "echo " + text}},
		},
	}
	line := map[string]any{"type": "assistant", "session_id": "thread", "message": message}
	if parent != "" {
		line["parent_tool_use_id"] = parent
		line["subagent_type"] = "general-purpose"
	}
	b, _ := json.Marshal(line)
	return b
}

func subagentResult(parent, id string) []byte {
	message := map[string]any{
		"role": "user",
		"content": []map[string]any{
			{"type": "tool_result", "tool_use_id": id, "content": "ok", "is_error": false},
		},
	}
	line := map[string]any{"type": "user", "session_id": "thread", "message": message}
	if parent != "" {
		line["parent_tool_use_id"] = parent
		line["subagent_type"] = "general-purpose"
	}
	b, _ := json.Marshal(line)
	return b
}

func agentOf(events []core.Event, kind string) []*core.EventAgent {
	var out []*core.EventAgent
	for _, e := range events {
		if e.Kind == kind {
			out = append(out, e.Agent)
		}
	}
	return out
}

func TestSubagentWorkIsRecordedAsTheirs(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.consume(subagentAssistant("toolu_alpha", "alpha", "ALPHA step 1"))
	s.consume(subagentResult("toolu_alpha", "toolu_alpha"))

	for _, kind := range []string{"assistant", "tool_call", "tool_result"} {
		for _, agent := range agentOf(s.log.All(), kind) {
			if agent == nil {
				t.Fatalf("a %s event from a subagent is recorded as the session's own", kind)
			}
			if agent.ID != "toolu_alpha" || agent.Type != "general-purpose" {
				t.Fatalf("%s: %+v", kind, agent)
			}
		}
	}
	// The existing phase marker stays, because readers use it.
	for _, e := range s.log.All() {
		if e.Kind == "assistant" && (e.Response == nil || e.Response.Phase != "subagent") {
			t.Fatal("the subagent phase was lost")
		}
	}
}

// Several subagents run inside one turn and share the session's run, so the
// turn is not what tells them apart. The call that started each one is.
func TestConcurrentSubagentsStayApart(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		s.consume(subagentAssistant("toolu_"+name, name, name+" step 1"))
	}
	seen := map[string]int{}
	for _, e := range s.log.All() {
		if e.Kind != "tool_call" {
			continue
		}
		if e.Agent == nil {
			t.Fatal("a subagent tool call lost its owner")
		}
		seen[e.Agent.ID]++
	}
	if len(seen) != 3 {
		t.Fatalf("three subagents collapsed into %d: %v", len(seen), seen)
	}
	for _, e := range s.log.All() {
		if e.Agent != nil && e.RunID != s.snap.RunID {
			t.Fatal("attribution replaced the run it belongs to")
		}
	}
}

// What the session does itself carries no owner. An empty one would be a claim
// that a subagent did it.
func TestTheSessionsOwnWorkHasNoOwner(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.consume(subagentAssistant("", "own", "a reply of my own"))
	s.consume(subagentResult("", "toolu_own"))
	for _, e := range s.log.All() {
		if e.Agent != nil {
			t.Fatalf("%s was attributed to %+v", e.Kind, e.Agent)
		}
	}
	// And the stash does not survive the envelope that set it.
	s.consume(subagentAssistant("toolu_alpha", "alpha", "ALPHA"))
	if s.subagent != nil {
		t.Fatal("the owner outlived the line that named it")
	}
	s.consume(subagentAssistant("", "after", "mine again"))
	for _, e := range s.log.All() {
		if e.Kind == "assistant" && e.Text == "mine again" && e.Agent != nil {
			t.Fatal("the next line inherited the subagent's owner")
		}
	}
}

// An id of "" is not an owner; recording it would make every event look like a
// subagent's with an unknown name.
func TestAnEmptyParentIsNotAnOwner(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	line := (`{"type":"assistant","session_id":"thread","parent_tool_use_id":"","message":{"model":"m","content":[{"type":"text","text":"hi"}]}}`)
	s.consume([]byte(line))
	for _, e := range s.log.All() {
		if e.Agent != nil {
			t.Fatalf("%s: %+v", e.Kind, e.Agent)
		}
	}
}

// The attribution survives the store, because a reader of a recorded
// conversation is who needs it.
func TestAttributionIsStored(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.consume(subagentAssistant("toolu_alpha", "alpha", "ALPHA"))
	for _, e := range s.log.All() {
		if e.Kind != "tool_call" {
			continue
		}
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		var back core.Event
		if err = json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		if back.Agent == nil || back.Agent.ID != "toolu_alpha" {
			t.Fatalf("round trip lost the owner: %s", b)
		}
		return
	}
	t.Fatal("no tool call was recorded")
}
