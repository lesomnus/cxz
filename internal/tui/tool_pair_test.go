package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func TestToolRowPendingWorkingDoneAtOriginalPosition(t *testing.T) {
	m := conversationModel()
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "run", RequestId: "tool", Kind: "tool_call", Text: "Edit", Payload: []byte(`{"file_path":"x.go","old_string":"a","new_string":"b\nc"}`)},
		{Seq: 2, RunId: "run", RequestId: "permission", Kind: "approval", Text: "Edit", Payload: []byte(`{"tool_use_id":"tool"}`)},
		{Seq: 3, RunId: "run", Kind: "assistant", Text: "AFTER_REQUEST"},
	}
	check := func(marker string) {
		m.render()
		text := ansi.Strip(m.view.View())
		if strings.Count(text, "Edit") != 1 || !strings.Contains(text, marker+" Edit x.go · +2 -1") || strings.Index(text, "Edit") > strings.Index(text, "AFTER_REQUEST") {
			t.Fatal(text)
		}
	}
	check("[ ]")
	m.events["s"] = append(m.events["s"], &api.Event{Seq: 4, RunId: "run", RequestId: "permission", Kind: "approval_resolved", Text: "allowed"})
	check("[•]")
	m.events["s"] = append(m.events["s"], &api.Event{Seq: 5, RunId: "run", RequestId: "tool", Kind: "tool_result", Payload: []byte(`{"content":"done"}`)})
	check("[✓]")
}

func TestUnpairedResultAndRunIsolation(t *testing.T) {
	m := conversationModel()
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "old", RequestId: "same", Kind: "tool_call", Text: "Bash", Payload: []byte(`{"command":"echo old"}`)},
		{Seq: 2, RunId: "run", RequestId: "same", Kind: "tool_result", Payload: []byte(`{"content":"UNPAIRED_RESULT"}`)},
	}
	m.render()
	text := ansi.Strip(m.view.View())
	if !strings.Contains(text, "[•] Bash") || !strings.Contains(text, "UNPAIRED_RESULT") {
		t.Fatal(text)
	}
}

func TestCodexUsesFinalFileDiffOnOriginalRow(t *testing.T) {
	m := conversationModel()
	m.sessions[0].Agent = "codex"
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "run", RequestId: "file", Kind: "tool_call", Payload: []byte(`{"item":{"type":"fileChange","changes":[]}}`)},
		{Seq: 2, RunId: "run", RequestId: "file", Kind: "tool_result", Payload: []byte(`{"item":{"type":"fileChange","status":"completed","changes":[{"path":"x.go","kind":{"type":"update"},"diff":"@@ -1 +1 @@\n-a\n+b\n"}]}}`)},
	}
	m.render()
	text := ansi.Strip(m.view.View())
	if strings.Count(text, "x.go") != 1 || !strings.Contains(text, "[✓] Edit x.go · +1 -1") {
		t.Fatal(text)
	}
}
