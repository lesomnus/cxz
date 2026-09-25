package supervisor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
)

func claudeSettingsReply(t *testing.T, s *Supervisor, model, effort string) {
	t.Helper()
	if s.settingsRequest == "" {
		t.Fatal("runtime did not query applied settings")
	}
	raw, _ := json.Marshal(map[string]any{"type": "control_response", "response": map[string]any{
		"request_id": s.settingsRequest, "subtype": "success", "response": map[string]any{"applied": map[string]string{"model": model, "effort": effort}},
	}})
	s.consume(raw)
}

func TestClaudeDefaultEffortIsConfirmedAndRestored(t *testing.T) {
	s, input := displaySupervisor(t, "claude")
	s.consume([]byte(`{"type":"control_response","response":{"request_id":"initialize","subtype":"success","response":{"models":[{"value":"default","resolvedModel":"claude-opus-5[1m]","supportedEffortLevels":["low","medium","high","xhigh","max"]}]}}}`))
	claudeSettingsReply(t, s, "claude-opus-5[1m]", "high")
	if s.appliedEffort != "high" || s.effort != "" {
		t.Fatal("default confused with an explicit preference")
	}
	for i, level := range []string{"low", "max", "default"} {
		id := fmt.Sprint(i)
		if _, err := s.execute("send", core.Command{ClientID: id, RunID: "run", Text: "/effort " + level}); err != nil {
			t.Fatal("cannot set reasoning without first selecting a model", err)
		}
		s.consume([]byte(fmt.Sprintf(`{"type":"control_response","response":{"request_id":"cxz-setting-%s","subtype":"success","response":{}}}`, id)))
		if s.settingPending != id {
			t.Fatal("acknowledgement alone confirmed effort")
		}
		if _, err := s.execute("send", core.Command{ClientID: "chat-" + id, RunID: "run", Text: "hello"}); err == nil {
			t.Fatal("chat started before effort was confirmed")
		}
		applied, preference := level, level
		if level == "default" {
			applied, preference = "high", ""
		}
		claudeSettingsReply(t, s, "claude-opus-5[1m]", applied)
		if s.effort != preference || s.appliedEffort != applied || s.settingPending != "" || s.receipts[id].Status != "accepted" {
			t.Fatal("reasoning state did not converge", level)
		}
		restored, _ := displaySupervisor(t, "claude")
		for _, event := range s.log.All() {
			if event.Kind == "setting" {
				restored.restoreSetting(event.Text, event.Payload)
			}
			if event.Kind == "input" || event.Kind == "turn_end" {
				t.Fatal("setting created a chat turn")
			}
		}
		args := claudeRunArgs(restored.session.Model, restored.effort, "resumed-thread")
		index := slices.Index(args, "--effort")
		if restored.effort != preference || preference != "" && (index < 0 || args[index+1] != preference) || preference == "" && index >= 0 {
			t.Fatal("restart lost reasoning preference", args)
		}
	}
	if !bytes.Contains(input.Bytes(), []byte(`"effortLevel":"max"`)) {
		t.Fatal("session-only max was not sent")
	}
}

func TestClaudeEffortRejectsUnverifiedOrDifferentAppliedLevel(t *testing.T) {
	for _, response := range []string{
		`{"subtype":"error","error":"unknown control"}`,
		`{"subtype":"success","response":{"effective":{"effortLevel":"high"}}}`,
		`{"subtype":"success","response":{"applied":{"model":"test","effort":"low"}}}`,
	} {
		s, _ := displaySupervisor(t, "claude")
		s.effort = "low"
		s.modelOptions = []agentview.ModelOption{{ID: "test", Default: true, Efforts: []string{"low", "high"}}}
		if _, err := s.execute("send", core.Command{ClientID: "effort", RunID: "run", Text: "/effort high"}); err != nil {
			t.Fatal(err)
		}
		s.consume([]byte(`{"type":"control_response","response":{"request_id":"cxz-setting-effort","subtype":"success","response":{}}}`))
		var result map[string]any
		_ = json.Unmarshal([]byte(response), &result)
		result["request_id"] = s.settingsRequest
		raw, _ := json.Marshal(map[string]any{"type": "control_response", "response": result})
		s.consume(raw)
		if s.effort != "low" || s.settingPending != "" || s.receipts["effort"].Status != "rejected" {
			t.Fatal("unverified setting was committed", response)
		}
	}
}

func TestClaudeSettingsIgnoreBackgroundReadsDuringChange(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.modelOptions = []agentview.ModelOption{{ID: "test", Default: true, Efforts: []string{"low", "high"}}}
	s.readClaudeSettings("")
	old := s.settingsRequest
	if _, err := s.execute("send", core.Command{ClientID: "effort", RunID: "run", Text: "/effort high"}); err != nil {
		t.Fatal(err)
	}
	s.consume([]byte(`{"type":"control_response","response":{"request_id":"cxz-setting-effort","subtype":"success","response":{}}}`))
	s.consume([]byte(fmt.Sprintf(`{"type":"control_response","response":{"request_id":%q,"subtype":"success","response":{"applied":{"model":"test","effort":"low"}}}}`, old)))
	if s.settingPending != "effort" || s.appliedEffort != "" {
		t.Fatal("stale state confirmed or replaced a new preference")
	}
	claudeSettingsReply(t, s, "test", "high")
	if s.effort != "high" {
		t.Fatal("current confirmation was lost")
	}
}
