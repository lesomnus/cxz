package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

func TestToolRowPendingWorkingDoneAtOriginalPosition(t *testing.T) {
	m := conversationModel()
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "run", RequestId: "tool", Kind: "tool_call", Text: "Edit", Payload: []byte(`{"file_path":"x.go","old_string":"a","new_string":"b\nc"}`)},
		{Seq: 2, RunId: "run", RequestId: "permission", Kind: "approval", Text: "Edit", Payload: []byte(`{"tool_use_id":"tool"}`)},
		{Seq: 3, RunId: "run", Kind: "assistant", Text: "AFTER_REQUEST"},
	}
	setRows(m, "s", m.events["s"])
	check := func(marker string) {
		m.render()
		text := ansi.Strip(m.view.View())
		if strings.Count(text, "Edit") != 1 || !strings.Contains(text, marker+" Edit x.go · +2 -1") || strings.Index(text, "Edit") > strings.Index(text, "AFTER_REQUEST") {
			t.Fatal(text)
		}
	}
	check("○")
	m.events["s"] = append(m.events["s"], &api.Event{Seq: 4, RunId: "run", RequestId: "permission", Kind: "approval_resolved", Text: "allowed"})
	setRows(m, "s", m.events["s"])
	check("•")
	m.events["s"] = append(m.events["s"], &api.Event{Seq: 5, RunId: "run", RequestId: "tool", Kind: "tool_result", Payload: []byte(`{"content":"done"}`)})
	setRows(m, "s", m.events["s"])
	check("✓")
}

func TestQueuedClaudeToolsDoNotStartBeforeTheirOwnApproval(t *testing.T) {
	m := conversationModel()
	m.current().State = "waiting_input"
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "run", RequestId: "first", Kind: "tool_call", Text: "Write", Payload: []byte(`{"file_path":"notes.md","content":"a"}`)},
		{Seq: 2, RunId: "run", RequestId: "second", Kind: "tool_call", Text: "Write", Payload: []byte(`{"file_path":"main.go","content":"b"}`)},
		{Seq: 3, RunId: "run", RequestId: "approval1", Kind: "approval", Text: "Write", Payload: []byte(`{"tool_use_id":"first"}`)},
	}
	setRows(m, "s", m.events["s"])
	check := func(first, second string) {
		m.render()
		text := ansi.Strip(m.view.View())
		if !strings.Contains(text, first+" Write notes.md") || !strings.Contains(text, second+" Write main.go") {
			t.Fatal(text)
		}
	}
	check("○", "○")
	m.events["s"] = append(m.events["s"], &api.Event{Seq: 4, RunId: "run", RequestId: "approval1", Kind: "approval_resolved", Text: "allowed"})
	setRows(m, "s", m.events["s"])
	check("•", "○")
	m.events["s"] = append(m.events["s"], &api.Event{Seq: 5, RunId: "run", RequestId: "first", Kind: "tool_result", Payload: []byte(`{"content":"done"}`)}, &api.Event{Seq: 6, RunId: "run", RequestId: "approval2", Kind: "approval", Text: "Write", Payload: []byte(`{"tool_use_id":"second"}`)})
	setRows(m, "s", m.events["s"])
	check("✓", "○")
	m.events["s"] = append(m.events["s"], &api.Event{Seq: 7, RunId: "run", RequestId: "approval2", Kind: "approval_resolved", Text: "allowed"})
	setRows(m, "s", m.events["s"])
	check("✓", "•")
}

func TestNativeExecutionSignalAndFailureColor(t *testing.T) {
	// A call that reports its own execution is working; one that only announces
	// an intention is pending. The runtime decides that now, so what is checked
	// here is that the row's state is drawn rather than re-derived.
	for _, in := range []struct{ state, want string }{{"working", "•"}, {"pending", "○"}, {"completed", "✓"}} {
		view := ansi.Strip(toolSummaryBody(&api.ToolSummary{Kind: "tool", Description: "Write", State: in.state}, 80, in.state))
		if !strings.Contains(view, in.want) {
			t.Fatalf("state %q drew %q", in.state, view)
		}
	}
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	for _, state := range []string{"failed", "denied", "canceled"} {
		view := toolSummaryBody(&api.ToolSummary{Kind: "tool", Description: "Write"}, 80, state)
		if !strings.Contains(view, sgr(failure)) || !strings.Contains(ansi.Strip(view), "×") {
			t.Fatal(view)
		}
	}
	for _, state := range []string{"denied", "canceled"} {
		view := approvalLine(&api.Session{Agent: "claude"}, &api.Event{Text: "Write"}, state, 80)
		if !strings.Contains(view, sgr(failure)) {
			t.Fatal(view)
		}
	}
}

// A request identity belongs to one run. The same id in another run is another
// tool, so the result opens its own row instead of closing the earlier call --
// which the runtime decides, and which no client has to notice.
func TestUnpairedResultAndRunIsolation(t *testing.T) {
	m := conversationModel()
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "old", RequestId: "same", Kind: "tool_call", Text: "Bash", Payload: []byte(`{"command":"echo old"}`)},
		{Seq: 2, RunId: "run", RequestId: "same", Kind: "tool_result", Text: "Bash", Payload: []byte(`{"content":"UNPAIRED_RESULT"}`)},
	}
	setRows(m, "s", m.events["s"])
	m.render()
	text := ansi.Strip(m.view.View())
	if rows := m.rows["s"]; len(rows) != 2 || rows[0].Seq != 1 || rows[1].Seq != 2 {
		t.Fatal("the result joined a call from another run", rows)
	}
	if !strings.Contains(text, "○ Bash") || !strings.Contains(text, "echo old") {
		t.Fatal("the unanswered call lost its own row", text)
	}
	if !strings.Contains(text, "✓ Bash") {
		t.Fatal("the orphan result did not read as finished", text)
	}
}

func TestCodexUsesFinalFileDiffOnOriginalRow(t *testing.T) {
	m := conversationModel()
	m.sessions[0].Agent = "codex"
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "run", RequestId: "file", Kind: "tool_call", Payload: []byte(`{"item":{"type":"fileChange","changes":[]}}`)},
		{Seq: 2, RunId: "run", RequestId: "file", Kind: "tool_result", Payload: []byte(`{"item":{"type":"fileChange","status":"completed","changes":[{"path":"x.go","kind":{"type":"update"},"diff":"@@ -1 +1 @@\n-a\n+b\n"}]}}`)},
	}
	setRows(m, "s", m.events["s"])
	m.render()
	text := ansi.Strip(m.view.View())
	if strings.Count(text, "x.go") != 1 || !strings.Contains(text, "✓ Edit x.go · +1 -1") {
		t.Fatal(text)
	}
}
