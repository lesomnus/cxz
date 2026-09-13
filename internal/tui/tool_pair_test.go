package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/muesli/termenv"
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

func TestQueuedClaudeToolsDoNotStartBeforeTheirOwnApproval(t *testing.T) {
	m := conversationModel()
	m.current().State = "waiting_input"
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "run", RequestId: "first", Kind: "tool_call", Text: "Write", Payload: []byte(`{"file_path":"notes.md","content":"a"}`)},
		{Seq: 2, RunId: "run", RequestId: "second", Kind: "tool_call", Text: "Write", Payload: []byte(`{"file_path":"main.go","content":"b"}`)},
		{Seq: 3, RunId: "run", RequestId: "approval1", Kind: "approval", Text: "Write", Payload: []byte(`{"tool_use_id":"first"}`)},
	}
	check := func(first, second string) {
		m.render()
		text := ansi.Strip(m.view.View())
		if !strings.Contains(text, first+" Write notes.md") || !strings.Contains(text, second+" Write main.go") {
			t.Fatal(text)
		}
	}
	check("[ ]", "[ ]")
	m.events["s"] = append(m.events["s"], &api.Event{Seq: 4, RunId: "run", RequestId: "approval1", Kind: "approval_resolved", Text: "allowed"})
	check("[•]", "[ ]")
	m.events["s"] = append(m.events["s"], &api.Event{Seq: 5, RunId: "run", RequestId: "first", Kind: "tool_result", Payload: []byte(`{"content":"done"}`)}, &api.Event{Seq: 6, RunId: "run", RequestId: "approval2", Kind: "approval", Text: "Write", Payload: []byte(`{"tool_use_id":"second"}`)})
	check("[✓]", "[ ]")
	m.events["s"] = append(m.events["s"], &api.Event{Seq: 7, RunId: "run", RequestId: "approval2", Kind: "approval_resolved", Text: "allowed"})
	check("[✓]", "[•]")
}

func TestNativeExecutionSignalAndFailureColor(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		e := &api.Event{Kind: "tool_call", Payload: []byte(`{"item":{"status":"inProgress"}}`)}
		want := "pending"
		if provider == "codex" {
			want = "working"
		}
		if toolInitialState(provider, e) != want {
			t.Fatal(provider)
		}
		if toolInitialState(provider, &api.Event{}) != "pending" {
			t.Fatal("unknown state treated as running")
		}
	}
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	for _, state := range []string{"failed", "denied", "canceled"} {
		view := toolActivityStateBody(agentview.ToolActivity{Kind: "tool", Description: "Write"}, nil, 80, state)
		if !strings.Contains(view, "38;2;242;109;120") || !strings.Contains(ansi.Strip(view), "[×]") {
			t.Fatal(view)
		}
	}
	for _, state := range []string{"denied", "canceled"} {
		view := approvalLine(&api.Session{Agent: "claude"}, &api.Event{Text: "Write"}, state, 80)
		if !strings.Contains(view, "38;2;242;109;120") {
			t.Fatal(view)
		}
	}
}

func TestUnpairedResultAndRunIsolation(t *testing.T) {
	m := conversationModel()
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "old", RequestId: "same", Kind: "tool_call", Text: "Bash", Payload: []byte(`{"command":"echo old"}`)},
		{Seq: 2, RunId: "run", RequestId: "same", Kind: "tool_result", Payload: []byte(`{"content":"UNPAIRED_RESULT"}`)},
	}
	m.render()
	text := ansi.Strip(m.view.View())
	if !strings.Contains(text, "[ ] Bash") || !strings.Contains(text, "UNPAIRED_RESULT") {
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
