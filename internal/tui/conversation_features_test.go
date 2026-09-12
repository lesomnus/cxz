package tui

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

func conversationModel() *model {
	m := projectModel()
	m.projectView = false
	m.input = newComposer()
	m.sessions = []*api.Session{{Id: "s", ProjectId: "p", RunId: "run", Agent: "claude", State: "idle"}}
	m.client = &recordingClient{}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

func TestProviderFixedBrandSlots(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	m := projectModel()
	m.openAccounts(false)
	m.accountAdding = true
	for _, agent := range []string{"claude", "codex"} {
		m.accountAgent = agent
		text := m.View()
		if !strings.Contains(text, providerLabel(agent)) {
			t.Fatal("brand lost", text)
		}
		for _, row := range strings.Split(ansi.Strip(text), "\n") {
			if i := strings.Index(row, "←"); i >= 0 && ansi.StringWidth(row[:i]) != 25 {
				t.Fatal("provider arrows moved", row)
			}
		}
	}
}

func TestApprovalFocusAndDecisionIdentity(t *testing.T) {
	m := conversationModel()
	c := m.client.(*recordingClient)
	m.current().Pending = []*api.Event{{RequestId: "one", Text: "Bash", Payload: []byte(`{"input":{"command":"pwd"}}`)}, {RequestId: "two", Text: "Write"}}
	m.resize()
	m.render()
	if !strings.Contains(m.View(), "Pending approvals") {
		t.Fatal(m.View())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !m.focusApproval || m.focusList {
		t.Fatal("Tab skipped approvals")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	// A later inventory cannot change the identity captured for this decision.
	m.current().Pending = []*api.Event{{RequestId: "new", Text: "Bash"}}
	msg := cmd()
	if len(c.answers) != 1 || c.answers[0].Allow || c.answers[0].RequestId != "two" || c.answers[0].RunId != "run" {
		t.Fatal(c.answers)
	}
	m.Update(msg)
	m.current().Pending = nil
	m.Update(listing{sessions: m.sessions})
	if m.focusApproval || !m.input.Focused() {
		t.Fatal("resolved approval left input blurred")
	}
	for _, size := range [][2]int{{40, 14}, {80, 24}, {120, 40}} {
		m.current().Pending = []*api.Event{{RequestId: "x", Text: "Bash", Payload: []byte(strings.Repeat("한글", 100))}}
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := ansi.Strip(m.View())
		rows := strings.Split(view, "\n")
		if len(rows) != size[1] {
			t.Fatal("wrong height", len(rows), size)
		}
		for _, row := range rows {
			if ansi.StringWidth(row) > size[0] {
				t.Fatal("overflow", row)
			}
		}
		if !strings.Contains(view, "Tab focus") {
			t.Fatal("approval controls hidden", view)
		}
	}
}

func TestPermissionScopeAndUnknownRequests(t *testing.T) {
	m := conversationModel()
	c := m.client.(*recordingClient)
	m.current().Pending = []*api.Event{{RequestId: "tool", Text: "Bash"}, {RequestId: "question", Text: "AskUserQuestion"}, {RequestId: "unknown", Text: "new/vendor/method"}}
	m.input.SetValue("/permission full")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("full did not approve pending tool")
	}
	msg := cmd()
	if len(c.inputs) != 0 || len(c.answers) != 1 || !c.answers[0].Allow {
		t.Fatal("incorrect full permission effect")
	}
	if m.autoApprove() != nil {
		t.Fatal("duplicate/question/unknown request auto-approved")
	}
	m.Update(msg)
	if m.autoApprove() != nil {
		t.Fatal("question auto-approved")
	}
	m.approvalID = "question"
	if m.replyApproval(m.selectedApproval(), true, "", false) != nil {
		t.Fatal("invented answer")
	}
	m.current().RunId = "new-run"
	m.current().Pending = []*api.Event{{RequestId: "new", Text: "Bash"}}
	if m.autoApprove() != nil {
		t.Fatal("permission leaked across runs")
	}
	m.permissionCommand("/permission full")
	m.Update(disconnected{id: "s"})
	if m.autoApprove() != nil || m.fullPermission["s"] != "" {
		t.Fatal("disconnect retained full permission")
	}
	m.permissionCommand("/permission full")
	m.permissionCommand("/permission ask")
	if m.autoApprove() != nil {
		t.Fatal("ask failed")
	}
	m.fullPermission = map[string]string{"s": "new-run"}
	m.Update(approvalResult{id: "s", err: errors.New("unknown outcome")})
	if m.fullPermission["s"] != "" || !strings.Contains(m.notice, "not retried") {
		t.Fatal("failed decision did not fail closed")
	}
}

func TestHistoryStickyPromptAndWorkingIndicator(t *testing.T) {
	m := conversationModel()
	m.events["s"] = []*api.Event{{Seq: 1, Kind: "input", Text: "First prompt\nsecond line\nthird line", TimeMs: 1000}, {Seq: 2, Kind: "assistant", Text: strings.Repeat("A response line\n", 60), TimeMs: 2000}}
	m.current().State = "working"
	m.render()
	m.view.GotoBottom()
	before := ansi.Strip(m.View())
	rows := strings.Split(before, "\n")
	if strings.TrimRight(rows[0], " ") != "> First prompt" || !strings.HasPrefix(rows[1], "  second line") || !strings.Contains(before, "⠋") || strings.Contains(before, "[working]") {
		t.Fatal(before)
	}
	m.Update(pulseTick{})
	if !strings.Contains(m.View(), "⠙") {
		t.Fatal("spinner did not animate")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	offset := m.view.YOffset
	if m.view.AtBottom() || !strings.Contains(m.View(), "L ") || !strings.Contains(m.View(), "01-01") {
		t.Fatal("missing scroll position/time", m.View())
	}
	m.Update(received{id: "s", event: &api.Event{Seq: 3, Kind: "assistant", Text: "Later response", TimeMs: 3000}})
	if m.view.YOffset != offset {
		t.Fatal("new output displaced reading position")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlEnd})
	if !m.view.AtBottom() {
		t.Fatal("did not follow latest")
	}
	m.Update(tea.MouseMsg{X: 2, Y: 2, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if m.view.AtBottom() {
		t.Fatal("mouse wheel did not scroll")
	}
	m.view.GotoBottom()
	m.current().State = "idle"
	m.render()
	if strings.Contains(m.View(), "⠙") || strings.Contains(m.View(), "[idle]") {
		t.Fatal("idle indicator remains")
	}
	m.view.GotoTop()
	if strings.Count(ansi.Strip(m.conversationView()), "> First prompt") != 1 {
		t.Fatal("prompt duplicated at top")
	}
	if len(m.historyTimes) != m.view.TotalLineCount() {
		t.Fatal("line metadata misaligned", len(m.historyTimes), m.view.TotalLineCount())
	}
}

func TestPinnedPromptFullRowBackground(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	const background = "48;2;3;30;44m"
	for _, width := range []int{40, 80, 123} {
		m := conversationModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		m.events["s"] = []*api.Event{
			{Seq: 1, Kind: "input", Text: "한글 prompt\nsecond line\nthird line"},
			{Seq: 2, Kind: "assistant", Text: strings.Repeat("Response\n", 60)},
		}
		m.render()
		m.view.GotoBottom()
		rows := strings.Split(m.View(), "\n")
		for i, row := range rows {
			if i < 2 {
				if ansi.StringWidth(row) != width || !strings.Contains(row, background) {
					t.Fatalf("width %d row %d not fully highlighted: %q", width, i, row)
				}
				// Padding must be inside the background span, before its reset.
				painted := strings.SplitN(row, background, 2)[1]
				painted = strings.SplitN(painted, "\x1b[0m", 2)[0]
				if ansi.StringWidth(painted) != width {
					t.Fatalf("unpainted padding: %q", row)
				}
			} else if strings.Contains(row, background) {
				t.Fatalf("background leaked outside pinned rows: %q", row)
			}
		}
		m.view.GotoTop()
		if strings.Contains(m.View(), background) {
			t.Fatal("prompt still highlighted when not pinned")
		}
	}
}

func TestCursorWriterNonTerminalDescriptor(t *testing.T) {
	w := &cursorWriter{out: &bytes.Buffer{}}
	if w.Fd() != ^uintptr(0) {
		t.Fatal("non-terminal writer must not borrow stdin or stdout")
	}
}

func TestMarkdownAndCodeSafety(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	input := "# Heading\n\n**bold** and `inline`\n\n- first\n- second\n\n```go\nfmt.Println(\"한글\")\n```\n\n[link](https://example.invalid)\n\x1b]52;c;secret\a"
	view := markdownView(input, 40)
	plain := ansi.Strip(view)
	for _, want := range []string{"Heading", "bold and inline", "• first", "fmt.Println(\"한글\")", "https://example.invalid"} {
		if !strings.Contains(plain, want) {
			t.Fatal(want, plain)
		}
	}
	if strings.Contains(plain, "```") || strings.Contains(view, "secret") || !strings.Contains(view, "48;2;0;0;0") {
		t.Fatal(view)
	}
	for _, row := range strings.Split(view, "\n") {
		if ansi.StringWidth(row) > 40 {
			t.Fatal("markdown overflow", row)
		}
	}
	if got := strings.TrimSpace(ansi.Strip(markdownView("Plain reply\nnext", 40))); got != "Plain reply\nnext" {
		t.Fatal("plain text changed", got)
	}
}

func TestTerminalCursorAnchor(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	m := conversationModel()
	var out bytes.Buffer
	w := &cursorWriter{out: &out}
	m.cursorOutput = w
	m.input.SetValue("한글 ㄱ")
	m.resize()
	m.View()
	if !w.enabled || w.x != 10 || w.y != 20 {
		t.Fatal("bad Unicode IME anchor", w.x, w.y, w.enabled)
	}
	w.Write([]byte("\x1b[?1049hframe"))
	if !strings.HasSuffix(out.String(), ansi.CursorPosition(w.x+1, w.y+1)) {
		t.Fatal("cursor not reanchored")
	}
	out.Reset()
	w.Write([]byte("\x1b[?1049llogin"))
	if out.String() != "\x1b[?1049llogin" {
		t.Fatal("external login cursor overridden")
	}
	for _, input := range []string{strings.Repeat("한글 ", 40), "one\ntwo\nthree\nfour\nfive\nsix\nseven\nㄱ"} {
		m.input.SetValue(input)
		m.resize()
		m.View()
		if !w.enabled || w.x < 3 || w.x >= 79 || w.y < 24-m.input.Height()-2 || w.y >= 22 {
			t.Fatal(fmt.Sprint("wrapped cursor outside composer: ", w.x, w.y), fmt.Sprintf("%q", m.input.View()))
		}
	}
}

func TestCursorAnchorWithoutColors(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(old)
	m := conversationModel()
	var out bytes.Buffer
	m.cursorOutput = &cursorWriter{out: &out}
	m.input.SetValue("ㄱ")
	m.resize()
	view := m.View()
	if !m.cursorOutput.enabled || m.cursorOutput.x != 5 {
		t.Fatal("NO_COLOR lost IME anchor")
	}
	if strings.Contains(view, "\x1b") {
		t.Fatal("cursor probe leaked into visible frame")
	}
}

func TestGFMAndResponseCache(t *testing.T) {
	raw := "| name | value |\n| --- | --- |\n| one | two |\n\n- [x] done\n\n~~removed~~"
	view := ansi.Strip(markdownView(raw, 60))
	for _, want := range []string{"name │ value", "one │ two", "☑ done", "removed"} {
		if !strings.Contains(view, want) {
			t.Fatal(want, view)
		}
	}
	m := conversationModel()
	e := &api.Event{Kind: "assistant", Text: "**cached**"}
	first := eventViewCached(m, m.current(), e, 60)
	if first != eventViewCached(m, m.current(), e, 60) || len(m.renderedResponses) != 1 {
		t.Fatal("cache miss")
	}
	e.Text = "different"
	if first == eventViewCached(m, m.current(), e, 60) {
		t.Fatal("stale response")
	}
}

func TestApprovalConversationPreview(t *testing.T) {
	m := conversationModel()
	m.current().Alias = "clover"
	m.current().Account = "work"
	m.current().Title = "Workspace UI"
	m.current().State = "waiting_input"
	m.current().Pending = []*api.Event{{RequestId: "one", Text: "Bash", Payload: []byte(`{"input":{"command":"go test ./..."}}`)}, {RequestId: "two", Text: "Write", Payload: []byte(`{"file_path":"README.md"}`)}}
	m.events["s"] = []*api.Event{{Seq: 1, Kind: "input", Text: "승인 화면을 확인해줘.", TimeMs: 1}, {Seq: 2, Kind: "assistant", Text: "## 검증\n\n- 입력 포커스 확인\n- `go test ./...` 실행 전 승인 요청", TimeMs: 2}}
	m.input.SetValue("한글 입력 ㄱ")
	m.resize()
	m.render()
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Enter allow") || !strings.Contains(view, "Backspace deny") {
		t.Fatal(view)
	}
	t.Log("\n" + view)
}
