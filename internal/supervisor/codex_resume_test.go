package supervisor

import (
	"encoding/json"
	"testing"
)

func TestCodexResumeExcludesConversationHistory(t *testing.T) {
	s, input := displaySupervisor(t, "codex")
	s.snap.State = "starting"
	s.modelDisabled = true
	s.codex.startThread()
	var request struct {
		Method string
		Params map[string]any
	}
	if err := json.Unmarshal(input.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	if request.Method != "thread/resume" || request.Params["threadId"] != "thread" || request.Params["excludeTurns"] != true {
		t.Fatalf("resume must request metadata without hydrating history: %+v", request)
	}
	// A metadata-only response must restore the same thread and become ready
	// without replaying old messages or requiring a turns page.
	s.consume([]byte(`{"id":"cxz-thread","result":{"thread":{"id":"thread","turns":[]}}}`))
	if s.snap.State != "idle" || s.snap.VendorID != "thread" {
		t.Fatalf("metadata-only resume did not become ready: %+v", s.snap)
	}
	for _, e := range s.log.All() {
		if e.Kind == "input" || e.Kind == "assistant" {
			t.Fatal("resume projected a historical message")
		}
	}
}

func TestCodexNewThreadDoesNotUseResumeParameters(t *testing.T) {
	s, input := displaySupervisor(t, "codex")
	s.snap.VendorID = ""
	s.modelDisabled = true
	s.codex.startThread()
	var request struct {
		Method string
		Params map[string]any
	}
	if err := json.Unmarshal(input.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	_, threadID := request.Params["threadId"]
	_, excludeTurns := request.Params["excludeTurns"]
	if request.Method != "thread/start" || threadID || excludeTurns {
		t.Fatalf("new thread received resume-only parameters: %+v", request)
	}
}
