package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
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
	if m.previewHeight() != 10 || m.previewSideWidth() != 0 {
		t.Fatal("narrow preview dimensions")
	}
	view := strings.Split(ansi.Strip(m.View()), "\n")
	if len(view) != m.height {
		t.Fatalf("height %d != %d", len(view), m.height)
	}
	top := m.view.Height + 2 + m.approvalHeight()
	if !strings.Contains(view[top+1], "Write") || !strings.Contains(view[top+10], "╭") {
		t.Fatalf("panel not above composer: %q", view)
	}
	if !m.filePreviewMouse(tea.MouseMsg{X: m.contentOffset() + 5, Y: top + 2, Button: tea.MouseButtonWheelDown}) || m.filePreview.offset != 3 {
		t.Fatal("scroll")
	}
	// The header close hitbox must match the rendered row on narrow screens.
	saved := m.filePreview
	if !m.filePreviewMouse(tea.MouseMsg{X: m.contentOffset() + m.width - 5, Y: top + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}) || m.filePreview != nil {
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
	x := m.contentOffset() + m.width + 2 + m.previewSideWidth() - 3
	if !m.filePreviewMouse(tea.MouseMsg{X: x, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}) || m.filePreview != nil {
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

func TestReadPreviewExpandsTabsBeforeMeasuring(t *testing.T) {
	profile := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(profile)
	source := "1\t#!/usr/bin/env python3\n2\t\"\"\"간단한 데모 스크립트.\"\"\"\n3\t\n4\timport sys\n5\tdef greet():\n6\t\tprint(\"안녕하세요\")"
	for _, color := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		lipgloss.SetColorProfile(color)
		for _, width := range []int{40, 80, 132} {
			m := conversationModel()
			m.filePreview = &filePreview{session: "s", title: "Read · /tmp/demo/greet.py", source: source, language: "greet.py", focused: true}
			rows := strings.Split(m.previewRows(width, 8), "\n")
			if len(rows) != 8 {
				t.Fatal("preview changed height")
			}
			for _, row := range rows {
				if strings.Contains(row, "\t") {
					t.Fatalf("unmeasured tab reaches terminal: %q", row)
				}
				if ansi.StringWidth(row) != width {
					t.Fatalf("row width %d != %d: %q", ansi.StringWidth(row), width, row)
				}
			}
			if m.filePreview.source != source {
				t.Fatal("display normalization changed recorded source")
			}
		}
	}
}

func TestPreviewBackgroundPaddingAndFocusRule(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	for _, width := range []int{40, 80} {
		for _, focused := range []bool{false, true} {
			m := conversationModel()
			m.filePreview = &filePreview{session: "s", title: "Read · demo.py", source: "1\tprint(\"한글\")\n2\t# comment", language: "demo.py", focused: focused}
			rendered := m.previewRows(width, 10)
			rows := strings.Split(ansi.Strip(rendered), "\n")
			if strings.TrimSpace(rows[0]) != "" || strings.ContainsAny(rendered, "╭╮╰╯") {
				t.Fatal("top padding or outer border")
			}
			if focused != strings.Contains(rows[9], "─") {
				t.Fatal("wrong focus indicator")
			}
			for i, row := range rows[:9] {
				if !strings.HasPrefix(row, " ") || !strings.HasSuffix(row, " ") {
					t.Fatalf("missing side padding at row %d: %q", i, row)
				}
			}
			terminal := vt.NewEmulator(width, 10)
			terminal.WriteString(strings.ReplaceAll(rendered, "\n", "\r\n"))
			for y := 0; y < 10; y++ {
				for x := 0; x < width; x++ {
					cell := terminal.CellAt(x, y)
					// The emulator stores a wide grapheme's style on its leading cell.
					if cell != nil && cell.Width == 0 && x > 0 {
						if leading := terminal.CellAt(x-1, y); leading != nil && leading.Width == 2 {
							cell = leading
						}
					}
					if cell == nil || cell.Style.Bg == nil {
						t.Fatal("unpainted preview cell", x, y)
					}
					r, g, b, _ := cell.Style.Bg.RGBA()
					if r != 0x3030 || g != 0x3030 || b != 0x3030 {
						t.Fatal("preview color differs from #303030", x, y)
					}
				}
			}
			terminal.Close()
		}
	}
}

func TestInlinePreviewOuterMargins(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	for _, width := range []int{40, 80, 133} {
		m := conversationModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		m.filePreview = &filePreview{session: "s", title: "Read · demo.py", source: strings.Repeat("x", 150), focused: true}
		m.resize()
		m.render()
		top := m.view.Height + 2 + m.approvalHeight()
		terminal := vt.NewEmulator(width, m.height)
		terminal.WriteString(strings.ReplaceAll(m.View(), "\n", "\r\n"))
		for y := top; y < top+m.previewHeight(); y++ {
			for x := m.contentOffset(); x < m.contentOffset()+m.width; x++ {
				cell := terminal.CellAt(x, y)
				if cell == nil {
					t.Fatal("missing cell", x, y)
				}
				margin := x < m.contentOffset()+2 || x >= m.contentOffset()+m.width-2
				if margin != (cell.Style.Bg == nil) {
					t.Fatal("incorrect preview margin background", x, y)
				}
			}
		}
		terminal.Close()
		if m.filePreviewMouse(tea.MouseMsg{X: m.contentOffset() + 1, Y: top + 2, Button: tea.MouseButtonWheelDown}) || m.filePreview.offset != 0 {
			t.Fatal("margin received preview scroll")
		}
	}
}
