package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

func TestFileSummaryCorrelatesInterleavedResults(t *testing.T) {
	m := conversationModel()
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "run", RequestId: "a", Kind: "tool_call", Text: "Write", Payload: []byte(`{"file_path":"/tmp/one.go","content":"SECRET_CONTENT\n"}`)},
		{Seq: 2, RunId: "run", RequestId: "b", Kind: "tool_call", Text: "Edit", Payload: []byte(`{"file_path":"/tmp/two.go","old_string":"old","new_string":"NEW_CONTENT","replace_all":true}`)},
		{Seq: 3, RunId: "run", RequestId: "a", Kind: "tool_result", Payload: []byte(`{"content":"VERBOSE_RESULT","is_error":false}`)},
		{Seq: 4, RunId: "run", RequestId: "b", Kind: "tool_result", Payload: []byte(`{"content":"ERROR_CONTENT","is_error":true}`)},
	}
	m.render()
	text := ansi.Strip(m.view.View())
	for _, want := range []string{"/tmp/one.go", "1 lines supplied", "/tmp/two.go", "per match; total unknown", "failed"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	for _, hidden := range []string{"SECRET_CONTENT", "NEW_CONTENT", "VERBOSE_RESULT", "ERROR_CONTENT", "file_path"} {
		if strings.Contains(text, hidden) {
			t.Fatal("raw file payload visible", text)
		}
	}
	m.toolDetails()
	if !strings.Contains(m.localReports["s"], "NEW_CONTENT") || !strings.Contains(m.localReports["s"], "ERROR_CONTENT") {
		t.Fatal("details lost raw input/result")
	}
}

func TestFileSummaryWithoutLoadedCallAndCommands(t *testing.T) {
	s := &api.Session{Agent: "codex"}
	e := &api.Event{Kind: "tool_result", Payload: []byte(`{"item":{"type":"fileChange","status":"completed","changes":[{"path":"main.go","kind":{"type":"add"},"diff":"@@ -0,0 +1 @@\n+HIDDEN_CONTENT\n"}]}}`)}
	text := ansi.Strip(eventView(s, e, 80))
	if !strings.Contains(text, "main.go") || !strings.Contains(text, "+1") || strings.Contains(text, "HIDDEN_CONTENT") {
		t.Fatal(text)
	}
	e = &api.Event{Kind: "tool_call", Text: "Bash", Payload: []byte(`{"command":"echo ok\nHIDDEN_BODY"}`)}
	text = ansi.Strip(eventView(&api.Session{Agent: "claude"}, e, 40))
	if !strings.Contains(text, "run · echo ok") || strings.Contains(text, "HIDDEN_BODY") {
		t.Fatal(text)
	}
}

func TestGreenBlinkingInputCursor(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	in := newComposer()
	in.Cursor.SetChar("x")
	in.Cursor.Blink = false
	if in.Cursor.Mode() != cursor.CursorBlink || !strings.Contains(in.Cursor.View(), "38;2;174;255;152") {
		t.Fatal("cursor not green/blinking")
	}
	in.Cursor.Blink = true
	if strings.Contains(in.Cursor.View(), "38;2;174;255;152") {
		t.Fatal("cursor color persists in blink-off phase")
	}
	confirm := newRecreateConfirmation()
	if len(strings.Split(confirm.View(), "\n")) > 7 {
		t.Fatal("recreate form too tall")
	}
}
