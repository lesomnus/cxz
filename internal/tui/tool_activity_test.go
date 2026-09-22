package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/muesli/termenv"
)

func TestWorkingToolDotBlinksWithoutRebuildingTranscript(t *testing.T) {
	m := conversationModel()
	m.current().State = "working"
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "run", Kind: "input", Text: "literal [•] Bash"},
		{Seq: 2, RunId: "run", Kind: "tool_call", RequestId: "running", Text: "Bash", Payload: []byte(`{"command":"printf '[•]'"}`)},
		{Seq: 3, RunId: "run", Kind: "approval", RequestId: "permission", Text: "Bash", Payload: []byte(`{"tool_use_id":"running"}`)},
		{Seq: 4, RunId: "run", Kind: "approval_resolved", RequestId: "permission", Text: "allowed"},
		{Seq: 5, RunId: "run", Kind: "tool_call", RequestId: "queued", Text: "Read", Payload: []byte(`{"file_path":"queued.txt"}`)},
	}
	m.render()
	baseline, tools := m.view.View(), len(m.renderedTools)
	on := ansi.Strip(m.conversationView())
	for range 5 { // The 100 ms tick changes the blink phase every half second.
		m.Update(pulseTick{})
	}
	off := ansi.Strip(m.conversationView())
	if !strings.Contains(on, "[•] Bash · printf '[•]'") || !strings.Contains(off, "[ ] Bash · printf '[•]'") {
		t.Fatalf("only the running header dot should blink:\non: %s\noff: %s", on, off)
	}
	for _, frame := range []string{on, off} {
		if !strings.Contains(frame, "literal [•] Bash") || !strings.Contains(frame, "[ ] Read queued.txt") {
			t.Fatal("animation changed user text or queued tool state", frame)
		}
	}
	if m.view.View() != baseline || len(m.renderedTools) != tools || ansi.StringWidth(on) != ansi.StringWidth(off) {
		t.Fatal("blink rebuilt or resized the cached transcript")
	}
	m.events["s"] = append(m.events["s"], &api.Event{Seq: 6, RunId: "run", Kind: "tool_result", RequestId: "running"})
	m.render()
	if len(m.workingToolRows) != 0 || !strings.Contains(ansi.Strip(m.conversationView()), "[✓] Bash") {
		t.Fatal("completed tool kept blinking")
	}
}

func TestCommandSummaryStaysWithinTwoRows(t *testing.T) {
	profile := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	commands := []string{
		"./" + strings.Repeat("x", 24) + " next command",
		`cd /go/pkg/mod/github.com/lesomnus/sqlite3-wasm@v0.0.0-20260907051834-c375b662b25b && ls; grep -rn "time\.\|_txlock\|Query()" --include='*.go' . | grep -v _test | head -20`,
		`cd /go/pkg/mod/github.com/lesomnus/sqlite3-wasm@v0.0.0-20260907051834-c375b662b25b && sed -n 1,25p time.go; grep -rn "time.Time" -A6 driver/conn.go | sed -n 1,30p`,
		"echo 한글🙂\n\tprintf '%s' 'second command'\necho third",
		"echo " + strings.Repeat("x", 250),
	}
	for _, profile := range []termenv.Profile{termenv.Ascii, termenv.ANSI256, termenv.TrueColor} {
		lipgloss.SetColorProfile(profile)
		for width := 38; width <= 133; width++ {
			for _, command := range commands {
				body := toolActivityBody(agentview.ToolActivity{Kind: "command", Command: command}, &api.Event{}, width)
				rows := strings.Split(ansi.Strip(body), "\n")
				if len(rows) > 2 {
					t.Fatalf("command rewrapped at width %d (%s): %q", width, profile.Name(), rows)
				}
				for _, row := range rows {
					if ansi.StringWidth(row) > width-2 || strings.Contains(row, "……") {
						t.Fatalf("command overflow or repeated truncation at width %d: %q", width, row)
					}
				}
				m := &model{}
				body = m.cachedToolBody("claude", &api.Event{}, agentview.ToolActivity{Kind: "command", Command: command}, nil, width, "working", true)
				rows = strings.Split(ansi.Strip(body), "\n")
				if len(rows) > 2 || !strings.HasSuffix(body, " · background") {
					t.Fatalf("background command lost its row limit or badge: %q", rows)
				}
				for _, row := range rows {
					if ansi.StringWidth(row) > width-2 {
						t.Fatalf("background badge overflows at width %d: %q", width, row)
					}
				}
			}
		}
	}
}

func TestLongCommandsKeepTranscriptAlignmentAfterResize(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := conversationModel()
	command := "cd /go/pkg/mod/github.com/lesomnus/sqlite3-wasm@v0.0.0-20260907051834-c375b662b25b && " + strings.Repeat("ls; ", 80)
	payload, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		t.Fatal(err)
	}
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "run", RequestId: "one", Kind: "tool_call", Text: "Bash", Payload: payload},
		{Seq: 2, RunId: "run", RequestId: "one", Kind: "tool_result"},
		{Seq: 3, RunId: "run", RequestId: "two", Kind: "tool_call", Text: "Bash", Payload: payload},
		{Seq: 4, RunId: "run", RequestId: "two", Kind: "tool_result"},
	}
	for _, width := range []int{69, 80, 110, 133, 170, 200, 80} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		if m.view.TotalLineCount() != 5 || len(m.historyPositions) != 5 {
			t.Fatalf("two previews and their separator changed height at width %d: %q", width, ansi.Strip(m.view.View()))
		}
		count := 0
		for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
			if i := strings.Index(row, "[✓] Bash"); i >= 0 {
				count++
				if ansi.StringWidth(row[:i]) != m.contentOffset()+2 {
					t.Fatalf("tool moved horizontally at width %d: %q", width, row)
				}
			}
		}
		if count != 2 {
			t.Fatal("tool disappeared after resize", width, count)
		}
	}
}

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
	for _, want := range []string{"[✓] Write /tmp/one.go", "+1 content", "[×] Edit /tmp/two.go", "+1 -1 /match"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	if strings.Count(text, "/tmp/one.go") != 1 || strings.Count(text, "/tmp/two.go") != 1 {
		t.Fatal("paired activity duplicated", text)
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
	e = &api.Event{Kind: "tool_call", Text: "Bash", Payload: []byte(`{"command":"echo ok\necho second\nHIDDEN_BODY"}`)}
	text = ansi.Strip(eventView(&api.Session{Agent: "claude"}, e, 40))
	if !strings.Contains(text, "Bash · echo ok") || !strings.Contains(text, "echo second") || strings.Contains(text, "HIDDEN_BODY") {
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
