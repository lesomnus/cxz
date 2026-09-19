package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

func TestCodeHighlightAndSanitize(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	for _, tc := range []struct{ code, lang string }{{"echo \"hello\" && exit 0", "bash"}, {"package main\nvar x = 1", "main.go"}, {"-old\n+new", "diff"}} {
		got := highlightCode(tc.code, tc.lang)
		if ansi.Strip(got) != tc.code || got == tc.code {
			t.Fatalf("highlight: %q", got)
		}
	}
	if got := highlightCode("echo \x1b]52;c;c2VjcmV0\a", "bash"); strings.Contains(got, "\x1b]52") {
		t.Fatal("unsafe terminal control")
	}
}
func TestFilePreviewLayoutAndScrolling(t *testing.T) {
	m := conversationModel()
	m.current().Agent = "claude"
	source := strings.Repeat("line\n", 20)
	m.events["s"] = []*api.Event{{Seq: 1, RunId: "run", Kind: "tool_call", Text: "Write", RequestId: "w", Payload: []byte(fmt.Sprintf(`{"file_path":"main.go","content":%q}`, source))}}
	m.render()
	m.view.GotoTop()
	row := -1
	for i, pos := range m.historyPositions {
		if int(pos) == 1 {
			row = i
			break
		}
	}
	if row < 0 || !m.openFilePreview(row) {
		t.Fatal("click did not open preview")
	}
	m.terminalWidth = 80
	m.width = 80
	m.resize()
	m.render()
	if m.previewHeight() != 8 || m.previewSideWidth() != 0 {
		t.Fatal("narrow preview dimensions")
	}
	view := strings.Split(ansi.Strip(m.View()), "\n")
	if len(view) != m.height {
		t.Fatalf("height %d != %d", len(view), m.height)
	}
	top := m.view.Height + 2 + m.approvalHeight()
	if !strings.Contains(view[top], "Write") || !strings.Contains(view[top+8], "╭") {
		t.Fatalf("panel not above composer: %q", view)
	}
	if !m.filePreviewMouse(tea.MouseMsg{X: 5, Y: top + 2, Button: tea.MouseButtonWheelDown}) || m.filePreview.offset != 3 {
		t.Fatal("scroll")
	}
	// The header close hitbox must match the rendered row on narrow screens.
	saved := m.filePreview
	if !m.filePreviewMouse(tea.MouseMsg{X: 79, Y: top, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}) || m.filePreview != nil {
		t.Fatal("narrow close hitbox")
	}
	m.filePreview = saved
	m.terminalWidth = 240
	m.width = maxViewWidth
	m.resize()
	m.render()
	if m.previewHeight() != 0 || m.previewSideWidth() < 40 {
		t.Fatal("wide preview dimensions")
	}
	if !strings.Contains(ansi.Strip(m.View()), "Write · main.go") {
		t.Fatal("missing side preview")
	}
	x := m.contentOffset() + m.width + 2 + m.previewSideWidth() - 1
	if !m.filePreviewMouse(tea.MouseMsg{X: x, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}) || m.filePreview != nil {
		t.Fatal("close")
	}
}
func TestCommandOutputTailAndCompletion(t *testing.T) {
	m := conversationModel()
	m.current().Agent = "codex"
	m.current().State = "working"
	m.events["s"] = []*api.Event{{Seq: 1, RunId: "run", Kind: "tool_call", RequestId: "c", Payload: []byte(`{"item":{"type":"commandExecution","command":"echo test","status":"inProgress"}}`)}, {Seq: 2, RunId: "run", Kind: "tool_output", RequestId: "c", Text: "1\n2\n3\n4\n5\n6\n7\n8\n"}}
	m.render()
	view := ansi.Strip(m.view.View())
	if strings.Contains(view, "│ 2") || !strings.Contains(view, "│ 8") {
		t.Fatal(view)
	}
	m.events["s"] = append(m.events["s"], &api.Event{Seq: 3, RunId: "run", Kind: "tool_result", RequestId: "c", Payload: []byte(`{"item":{"type":"commandExecution","command":"echo test","status":"completed"}}`)})
	m.render()
	if strings.Contains(ansi.Strip(m.view.View()), "│ 8") {
		t.Fatal("completed command retained live output")
	}
	if len(outputTail(strings.Repeat("x", 100000))) > 32768 {
		t.Fatal("unbounded tail")
	}
}
func TestSelectedQuestionOptionMovesToSubmit(t *testing.T) {
	m := questionModel()
	m.syncQuestion()
	d := m.questionDialog
	d.questions = d.questions[:1]
	d.questions[0].Multi = false
	d.row = 0
	m.questionKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !d.selected[0][0] || d.row != 0 {
		t.Fatal("first selection")
	}
	if cmd := m.questionKey(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil || d.row != d.count() || d.sending {
		t.Fatal("second selection must focus, not submit")
	}
}
func TestQuotaRetainsNumericSnapshotAndSharesAccount(t *testing.T) {
	now := time.Now().UnixMilli()
	m := conversationModel()
	m.current().Agent = "claude"
	m.current().Account = "same"
	m.sessions = append(m.sessions, &api.Session{Id: "peer", Agent: "claude", Account: "same", RunId: "r2"}, &api.Session{Id: "other", Agent: "claude", Account: "different", RunId: "r3"})
	m.events["peer"] = []*api.Event{{Kind: "usage", Text: "get_usage", RunId: "r2", TimeMs: now, Payload: []byte(`{"rate_limits":{"five_hour":{"utilization":30}}}`)}}
	m.events["s"] = []*api.Event{{Kind: "usage", Text: "get_usage", RunId: "run", TimeMs: now + 1, Payload: []byte(`{"rate_limits":null}`)}, {Kind: "usage", Text: "rate_limit_event", RunId: "run", TimeMs: now + 2, Payload: []byte(`{"rate_limit_info":{"rateLimitType":"five_hour","status":"allowed"}}`)}}
	m.events["other"] = []*api.Event{{Kind: "usage", Text: "get_usage", RunId: "r3", TimeMs: now + 3, Payload: []byte(`{"rate_limits":{"five_hour":{"utilization":90}}}`)}}
	m.updateQuota()
	if len(m.quotaWindows) != 1 || m.quotaWindows[0].Remaining == nil || *m.quotaWindows[0].Remaining != 70 {
		t.Fatalf("quota lost or crossed accounts: %+v", m.quotaWindows)
	}
}
func TestCatchupPageRejectsStaleAndDuplicateEvents(t *testing.T) {
	m := conversationModel()
	m.watchEpoch = 2
	events := []*api.Event{{Seq: 1, Kind: "assistant", Text: "first"}, {Seq: 2, Kind: "assistant", Text: "second"}}
	m.Update(caughtUp{id: "s", events: events, epoch: 1})
	if len(m.events["s"]) != 0 {
		t.Fatal("stale catchup applied")
	}
	m.Update(caughtUp{id: "s", events: events, epoch: 2})
	m.Update(caughtUp{id: "s", events: events, epoch: 2})
	if len(m.events["s"]) != 2 || m.cursor["s"] != 2 {
		t.Fatal("catchup duplicated or lost events")
	}
}

func TestEditPreviewUsesRecordedReplacement(t *testing.T) {
	p := previewFor("claude", &api.Event{Text: "Edit", Payload: []byte(`{"file_path":"gone.go","old_string":"old\nline","new_string":"new"}`)})
	if p == nil || p.source != "-old\n-line\n+new" || p.language != "diff" {
		t.Fatalf("%+v", p)
	}
	p = previewFor("codex", &api.Event{Payload: []byte(`{"item":{"type":"fileChange","changes":[{"path":"one.go","diff":"@@ -1 +1 @@\n-a\n+b"}]}}`)})
	if p == nil || !strings.Contains(p.source, "one.go\n@@ -1 +1 @@") {
		t.Fatalf("%+v", p)
	}
}
func TestMultiChoiceEnterFocusesSubmitButSpaceToggles(t *testing.T) {
	m := questionModel()
	m.syncQuestion()
	d := m.questionDialog
	d.questions[0].Multi = true
	space := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	m.questionKey(space)
	m.questionKey(space)
	if d.selected[0][0] || d.row != 0 {
		t.Fatal("Space must still deselect")
	}
	m.questionKey(tea.KeyMsg{Type: tea.KeyEnter})
	m.questionKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !d.selected[0][0] || d.row != d.count() || d.sending {
		t.Fatal("Enter must focus Next without submitting")
	}
}
func TestQuotaCacheSurvivesTailHistoryGap(t *testing.T) {
	m := conversationModel()
	m.current().Agent = "claude"
	m.current().Account = "shared"
	m.events["s"] = []*api.Event{{Kind: "usage", Text: "get_usage", RunId: "run", TimeMs: 1000, Payload: []byte(`{"rate_limits":{"five_hour":{"utilization":20}}}`)}}
	m.updateQuota()
	m.current().RunId = "new-run"
	m.events["s"] = nil
	m.updateQuota()
	if len(m.quotaWindows) != 1 || *m.quotaWindows[0].Remaining != 80 || m.quotaWindows[0].Observed.UnixMilli() != 1000 {
		t.Fatal("last known account quota lost or timestamp refreshed")
	}
	m.current().Account = "other"
	m.updateQuota()
	if len(m.quotaWindows) != 0 {
		t.Fatal("cached quota crossed accounts")
	}
}
