package supervisor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
)

const asyncQuestionEvent = `{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"call_q","text":"Theme?","delivery":"async","questions":[{"title":"Theme?","options":["Light","Dark"]}]}}}`

func TestCodexAsyncQuestionSurvivesTurnAndRepliesAsToolOutput(t *testing.T) {
	s, input := displaySupervisor(t, "codex")
	s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"turn"}}}`))
	s.consume([]byte(asyncQuestionEvent))
	if s.snap.State != "working" || s.pending["async:call_q"].Text != agentview.CodexAsyncQuestion {
		t.Fatal("async question blocked or lost")
	}
	s.consume([]byte(`{"id":42,"method":"item/tool/requestUserInput","params":{"questions":[{"id":"q","question":"Blocking?"}]}}`))
	s.consume([]byte(`{"method":"turn/completed","params":{"turn":{"id":"turn","status":"completed"}}}`))
	if s.snap.State != "idle" || len(s.pending) != 1 {
		t.Fatal("turn cleanup discarded async question")
	}
	input.Reset()
	command := core.Command{RunID: "run", ClientID: "answer", RequestID: "async:call_q", Allow: true, Selections: map[string]core.AnswerSelection{"0": {Selected: []string{"Dark"}}}}
	if _, err := s.execute("reply", command); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Method string
		Params struct {
			Input      []any
			ToolOutput struct{ Name, Output string }
		}
	}
	if err := json.Unmarshal(input.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Method != "turn/start" || len(wire.Params.Input) != 0 || wire.Params.ToolOutput.Name != "request_user_input_async" || !strings.Contains(wire.Params.ToolOutput.Output, `"selected":["Dark"]`) {
		t.Fatal(input.String())
	}
	before := input.Len()
	s.execute("reply", command)
	if input.Len() != before {
		t.Fatal("duplicate answer sent")
	}
	s.consume([]byte(asyncQuestionEvent))
	if len(s.pending) != 0 {
		t.Fatal("replayed question reopened")
	}
	s.consume([]byte(`{"id":"answer","error":{"code":-32602,"message":"unsupported"}}`))
	if len(s.pending) != 1 {
		t.Fatal("rejected answer was lost")
	}
}

func TestCodexAsyncDismissAndValidation(t *testing.T) {
	s, input := displaySupervisor(t, "codex")
	s.consume([]byte(asyncQuestionEvent))
	bad := core.Command{RunID: "run", ClientID: "bad", RequestID: "async:call_q", Allow: true, Selections: map[string]core.AnswerSelection{"0": {Selected: []string{"Light", "Dark"}}}}
	if _, err := s.execute("reply", bad); err == nil {
		t.Fatal("unsupported multiselect accepted")
	}
	if len(s.pending) != 1 || input.Len() != 0 {
		t.Fatal("validation mutated pending or wrote")
	}
	if _, err := s.execute("reply", core.Command{RunID: "run", ClientID: "dismiss", RequestID: "async:call_q"}); err != nil {
		t.Fatal(err)
	}
	if s.snap.State != "idle" || input.Len() != 0 || len(s.pending) != 0 {
		t.Fatal("dismissal started a turn")
	}
}

func TestAsyncPendingDoesNotKeepToolApprovalBlocked(t *testing.T) {
	s, _ := displaySupervisor(t, "codex")
	s.consume([]byte(asyncQuestionEvent))
	s.consume([]byte(`{"id":42,"method":"item/commandExecution/requestApproval","params":{"command":"echo ok"}}`))
	if _, err := s.execute("reply", core.Command{RunID: "run", ClientID: "allow", RequestID: "42", Allow: true}); err != nil {
		t.Fatal(err)
	}
	if s.snap.State != "working" || len(s.pending) != 1 {
		t.Fatal("nonblocking question held foreground in waiting_input")
	}
}
