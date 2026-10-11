package agentview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func TestQuestionReplyHistory(t *testing.T) {
	raw := []byte(`{"params":{"questions":[{"id":"a","question":"Choose","options":[{"label":"One"}]},{"id":"secret","question":"Token?","isSecret":true}]}}`)
	selections := map[string]core.AnswerSelection{"a": {Selected: []string{"One"}}, "secret": {Other: "private-token"}}
	got := QuestionReply("codex", "item/tool/requestUserInput", raw, "allowed", nil, selections)
	encoded, _ := json.Marshal(got)
	if got == nil || got.State != "allowed" || got.Answers["a"].Selected[0] != "One" || strings.Contains(string(encoded), "private-token") || got.Answers["secret"].Other != "[redacted]" {
		t.Fatal(string(encoded))
	}
	if selections["secret"].Other != "private-token" {
		t.Fatal("mutated provider reply")
	}
	if got := QuestionReply("codex", "item/tool/requestUserInput", raw, "denied", nil, selections); got == nil || len(got.Answers) > 0 {
		t.Fatal(got)
	}
	if got := QuestionReply("claude", "Bash", []byte(`{"input":{"command":"pwd"}}`), "allowed", nil, nil); got != nil {
		t.Fatal(got)
	}
	legacy := QuestionReply("codex", "item/tool/requestUserInput", raw, "allowed", map[string]string{"a": "legacy", "secret": "private"}, nil)
	if legacy.Answers["a"].Other != "legacy" || legacy.Answers["secret"].Other != "[redacted]" {
		t.Fatal(legacy)
	}
}
