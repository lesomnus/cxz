package tui

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

// Find a visible control in the actual composed screen, including project-panel
// offsets, rather than reusing the hit-test geometry to manufacture coordinates.
func questionPoint(t *testing.T, m *model, label string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if i := strings.Index(line, label); i >= 0 {
			return ansi.StringWidth(line[:i]), y
		}
	}
	t.Fatalf("missing visible control %q: %s", label, ansi.Strip(m.View()))
	return 0, 0
}

func TestQuestionDescriptionHangingIndent(t *testing.T) {
	for _, width := range []int{40, 110, 240} {
		m := questionModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
		m.syncQuestion()
		d := m.questionDialog
		d.questions[0].Options[0].Label = strings.Repeat("긴 선택지 ", 15) + "\nLABEL_CONTINUATION"
		d.questions[0].Options[0].Description = "HARDENING.md " + strings.Repeat("측정 기록과 수용 조건 ", 15) + "\nEXPLICIT_CONTINUATION"
		d.questions[0].Options[0].Preview = ""
		lines, count := m.questionLayout().lines, 0
		for _, line := range lines {
			if line.item != 0 {
				continue
			}
			plain := ansi.Strip(line.text)
			if count > 0 && !strings.HasPrefix(plain, "    ") {
				t.Fatal("wrapped label/description lost indentation", width, plain)
			}
			count++
		}
		if count < 5 {
			t.Fatal("fixture did not wrap")
		}
		for _, line := range strings.Split(m.View(), "\n") {
			if ansi.StringWidth(line) != width {
				t.Fatal("wrapped text overflowed terminal", width, line)
			}
		}
	}
}

func TestQuestionHoverFullRowsAndKeyboardRestore(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	for _, width := range []int{40, 110, 240} {
		m := questionModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
		m.syncQuestion()
		d := m.questionDialog
		d.questions[0].Options[0].Preview = ""
		d.questions[0].Options[1].Description = "second description\nsecond continuation"
		x, y := questionPoint(t, m, "○ B")
		_, ay := questionPoint(t, m, "○ A")
		terminal := vt.NewEmulator(width, 40)
		terminal.WriteString(strings.ReplaceAll(m.View(), "\n", "\r\n"))
		left, right := m.contentOffset()+3, m.contentOffset()+m.width-3
		for col := left; col < right; col++ {
			if cell := terminal.CellAt(col, ay); cell == nil || cell.Style.Bg == nil {
				t.Fatal("keyboard row does not fill box", width, col)
			}
		}
		keyboard := terminal.CellAt(left, ay).Style.Bg
		terminal.Close()
		m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion})
		if d.row != 0 || d.activeRow() != 1 || d.selected[0][1] {
			t.Fatal("hover committed keyboard position or an answer")
		}
		m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
		if d.row != 0 || d.selected[0][1] {
			t.Fatal("release without a press selected an answer")
		}
		terminal = vt.NewEmulator(width, 40)
		terminal.WriteString(strings.ReplaceAll(m.View(), "\n", "\r\n"))
		for row := y; row <= y+2; row++ {
			for col := left; col < right; col++ {
				cell := terminal.CellAt(col, row)
				if cell == nil || cell.Style.Bg != keyboard {
					t.Fatal("hover/description fill differs from keyboard", width, col, row)
				}
			}
			if terminal.CellAt(left-1, row).Style.Bg != nil || terminal.CellAt(right, row).Style.Bg != nil {
				t.Fatal("selection painted the borders")
			}
		}
		if terminal.CellAt(left, ay).Style.Bg != nil {
			t.Fatal("keyboard highlight remains during hover")
		}
		terminal.Close()
		m.Update(tea.MouseMsg{X: x, Y: 0, Action: tea.MouseActionMotion})
		if d.activeRow() != 0 || d.row != 0 {
			t.Fatal("leaving hover did not restore keyboard row")
		}
		m.Update(tea.MouseMsg{X: x, Y: y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if d.row != 1 || !d.selected[0][1] {
			t.Fatal("description click did not select its option")
		}
		m.Update(tea.MouseMsg{X: x, Y: 0, Action: tea.MouseActionMotion})
		if d.activeRow() != 1 {
			t.Fatal("click did not become new keyboard position")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyUp})
		if d.activeRow() != 0 {
			t.Fatal("keyboard cannot take over after mouse")
		}
	}
}

func TestQuestionDockAndIndependentScrolling(t *testing.T) {
	for _, width := range []int{40, 110, 240} {
		m := questionModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 36})
		m.events["s"] = []*api.Event{{Seq: 1, Kind: "assistant", Text: strings.Repeat("history line\n", 100) + "LAST_REPLY"}}
		m.render()
		m.view.GotoBottom()
		before := m.view.Height
		m.syncQuestion()
		d := m.questionDialog
		d.questions[0].Options[0].Description = strings.Repeat("long description\n", 50) + "LAST_DESCRIPTION"
		d.questions[0].Options[0].Preview = ""
		view := strings.Split(m.View(), "\n")
		conversation := strings.Split(m.conversationView(), "\n")
		if len(view) != m.height || m.view.Height < 1 || m.view.Height >= before {
			t.Fatal("question did not reserve transcript space")
		}
		for y := 0; y < m.view.Height; y++ {
			actual := ansi.Strip(ansi.Cut(view[y], m.contentOffset(), m.contentOffset()+m.width))
			if actual != ansi.Strip(conversation[y]) {
				t.Fatal("question covered the conversation", y, actual)
			}
		}
		if !strings.Contains(ansi.Strip(strings.Join(view[:m.view.Height], "\n")), "LAST_REPLY") {
			t.Fatal("latest reply hidden by question")
		}
		yOffset := m.view.YOffset
		m.Update(tea.MouseMsg{X: m.contentOffset() + 5, Y: 1, Button: tea.MouseButtonWheelUp})
		if m.view.YOffset >= yOffset || d.offset != 0 {
			t.Fatal("conversation wheel went to question")
		}
		yOffset = m.view.YOffset
		x, y := questionPoint(t, m, "○ A")
		m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonWheelDown})
		if d.offset == 0 || m.view.YOffset != yOffset {
			t.Fatal("question wheel changed conversation")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEnd})
		if !strings.Contains(m.View(), "LAST_DESCRIPTION") || !strings.Contains(m.View(), "[ Next ]") {
			t.Fatal("description end or pinned buttons inaccessible")
		}
		offset := d.offset
		m.Update(tea.KeyMsg{Type: tea.KeyPgUp, Alt: true})
		if m.view.YOffset >= yOffset || d.offset != offset {
			t.Fatal("modified PgUp did not scroll conversation independently")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyHome})
		if d.offset != 0 || !strings.Contains(m.View(), "무엇을?") {
			t.Fatal("cannot scroll question to beginning")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if m.view.Height != before {
			t.Fatal("closing question did not return reserved space")
		}
	}
}

func TestQuestionMouseButtonsOtherAndSubmission(t *testing.T) {
	m := questionModel()
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m.syncQuestion()
	d := m.questionDialog
	click := func(label string) tea.Cmd {
		x, y := questionPoint(t, m, label)
		_, cmd := m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		return cmd
	}
	click("○ B")
	click("[ Next ]")
	if d.page != 1 || !d.selected[0][1] {
		t.Fatal("Next lost selection")
	}
	click("[ ] X")
	click("[✓] X")
	if d.selected[1][0] {
		t.Fatal("mouse cannot toggle a checkbox off")
	}
	click("[ ] Y")
	click("Other:")
	m.Update(questionKeyMsg("custom text"))
	if d.other[1].Value() != "custom text" || !d.selected[1][1] || !d.otherSelected[1] {
		t.Fatal("Other click did not allow editing alongside multi-select")
	}
	click("[ Back ]")
	if d.page != 0 || !d.selected[0][1] || d.other[1].Value() != "custom text" {
		t.Fatal("Back lost answers")
	}
	click("[ Next ]")
	cmd := click("[ Submit ]")
	if cmd == nil || !d.sending {
		t.Fatal("Submit click did not send")
	}
	if again := click("[ Submit ]"); again != nil {
		t.Fatal("mouse double submission")
	}
	cmd()
	if len(m.client.(*recordingClient).answers) != 1 {
		t.Fatal("answer not sent exactly once")
	}
}

func TestQuestionOtherCursorFollowsDockAndHover(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := questionModel()
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m.syncQuestion()
	d := m.questionDialog
	d.row, d.reveal = len(d.questions[0].Options), true
	m.cursorOutput = &cursorWriter{out: &bytes.Buffer{}}
	_, y := questionPoint(t, m, "Other:")
	m.anchorCursor()
	if !m.cursorOutput.enabled || m.cursorOutput.y != y {
		t.Fatal("Other cursor not anchored in docked panel", m.cursorOutput.y, y)
	}
	x, optionY := questionPoint(t, m, "○ B")
	m.Update(tea.MouseMsg{X: x, Y: optionY, Action: tea.MouseActionMotion})
	m.anchorCursor()
	if m.cursorOutput.enabled {
		t.Fatal("text cursor remains active during option hover")
	}
	m.Update(tea.MouseMsg{X: x, Y: 0, Action: tea.MouseActionMotion})
	m.anchorCursor()
	if !m.cursorOutput.enabled || d.row != len(d.questions[0].Options) {
		t.Fatal("leaving hover did not restore Other cursor")
	}
}

func TestQuestionSmallScreensOtherPanelsAndCancel(t *testing.T) {
	for _, size := range [][2]int{{40, 14}, {110, 24}, {240, 50}} {
		m := questionModel()
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.input.SetValue("saved\ndraft\nwith lines")
		m.current().PermissionMode = "full"
		m.terminals = map[string]*terminalPanel{"s": {open: true, focused: true}}
		m.filePreview = &filePreview{session: "s", source: "file", title: "Read", focused: true}
		m.showError("previous operation failed")
		m.openQuestion(m.selectedApproval())
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		if len(rows) != size[1] || m.view.Height < 1 || !strings.Contains(rows[len(rows)-1], "FULL") || !strings.Contains(strings.Join(rows, "\n"), "[ Cancel ]") {
			t.Fatal("question or composer displaced by another panel", size, rows)
		}
		for _, row := range rows {
			if ansi.StringWidth(row) != size[0] {
				t.Fatal("question row overflow", size, row)
			}
		}
		if m.terminalFocused() || m.filePreview.focused || m.input.Focused() {
			t.Fatal("question did not take keyboard focus")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
		if m.questionDialog.row != 1 {
			t.Fatal("another panel intercepted question navigation")
		}
		x, y := questionPoint(t, m, "[ Cancel ]")
		m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.questionDialog != nil || !m.input.Focused() || m.input.Value() != "saved\ndraft\nwith lines" || len(m.client.(*recordingClient).answers) != 0 {
			t.Fatal("Cancel sent an answer or lost the draft/focus")
		}
		if !m.errorVisible() {
			t.Fatal("compact layout lost retained error")
		}
	}
}
