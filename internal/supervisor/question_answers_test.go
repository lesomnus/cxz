package supervisor

import (
	"encoding/json"
	"github.com/lesomnus/cxz/internal/core"
	"strings"
	"testing"
)

func TestClaudeStructuredQuestionReply(t *testing.T) {
	s, input := displaySupervisor(t, "claude")
	s.pending["q"] = core.Event{Payload: []byte(`{"tool_name":"AskUserQuestion","input":{"questions":[{"question":"Many?","multiSelect":true,"options":[{"label":"low"},{"label":"a, b"}]},{"question":"One?","options":[{"label":"high","preview":"preview content"}]}]}}`)}
	cmd := core.Command{RunID: "run", ClientID: "choice", RequestID: "q", Allow: true, Selections: map[string]core.AnswerSelection{"Many?": {Selected: []string{"low", "a, b"}, Other: "  custom, text  "}, "One?": {Selected: []string{"high"}}}}
	if _, err := s.execute("reply", cmd); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Response struct {
			Response struct {
				UpdatedInput struct {
					Answers     map[string]string
					Annotations map[string]map[string]string
				}
			}
		}
	}
	if err := json.Unmarshal(input.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	got := wire.Response.Response.UpdatedInput
	if got.Answers["Many?"] != "low, a, b, custom, text" || got.Annotations["One?"]["preview"] != "preview content" {
		t.Fatalf("%+v", got)
	}
	notes := strings.TrimPrefix(got.Annotations["Many?"]["notes"], "cxz structured selection: ")
	var exact core.AnswerSelection
	if json.Unmarshal([]byte(notes), &exact) != nil || len(exact.Selected) != 2 || exact.Selected[1] != "a, b" || exact.Other != "custom, text" {
		t.Fatal(notes)
	}
	before := input.Len()
	if _, err := s.execute("reply", cmd); err != nil || input.Len() != before {
		t.Fatal("idempotent retry changed", err)
	}
}

func TestCodexStructuredQuestionReply(t *testing.T) {
	s, input := displaySupervisor(t, "codex")
	s.pending[`"q"`] = core.Event{Payload: []byte(`{"id":"q","method":"item/tool/requestUserInput","params":{"questions":[{"id":"choice","question":"Pick","options":[{"label":"a, b"}]}]}}`)}
	cmd := core.Command{RunID: "run", ClientID: "choice", RequestID: `"q"`, Allow: true, Selections: map[string]core.AnswerSelection{"choice": {Selected: []string{"a, b"}}}}
	if _, err := s.execute("reply", cmd); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Result struct {
			Answers map[string]struct{ Answers []string }
		}
	}
	if json.Unmarshal(input.Bytes(), &wire) != nil {
		t.Fatal(input.String())
	}
	a := wire.Result.Answers["choice"].Answers
	if len(a) != 1 || a[0] != "a, b" {
		t.Fatal(a)
	}
}
