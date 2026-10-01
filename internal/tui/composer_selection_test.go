package tui

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestComposerShiftSelectionEditing(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("hello 한글")
	m.input.CursorEnd()
	for range 2 {
		m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	}
	if got := m.selectedComposerText(); got != "한글" {
		t.Fatalf("selected %q", got)
	}
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	view := m.composerSelectionView(m.input.View())
	if !strings.Contains(view, "48;5;240") || !strings.Contains(ansi.Strip(view), "hello 한글") {
		t.Fatalf("selection not painted: %q", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("수정")})
	if got := m.input.Value(); got != "hello 수정" {
		t.Fatalf("replacement: %q", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := m.input.Value(); got != "hello 수" {
		t.Fatalf("deletion: %q", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if composerPosition(m.input) != 6 || m.selectedComposerText() != "" {
		t.Fatal("left did not collapse selection")
	}
}

func TestComposerMouseCursorMapping(t *testing.T) {
	m := conversationModel()
	m.width = 28
	m.input.SetValue("한글 hello world words wrap here\nsecond line\nthird\nfourth\nfifth\nsixth\nlast")
	m.resize()
	// Compare hit testing against the textarea's rendered cursor, including its
	// private scroll offset, soft wraps, CJK widths and the sidebar offset.
	for _, pos := range []int{0, 1, 2, 5, 12, 20, 30, 38, 55, 65} {
		m.setComposerPosition(pos)
		m.resize()
		probe := m.input
		probe.Cursor.Blink = false
		probe.Cursor.Style = cursorProbeStyle
		x, y, ok := widgetCursor(probe.View())
		if !ok {
			t.Fatal("cursor unavailable")
		}
		got := m.composerPoint(x-2, y)
		if got != composerPosition(m.input) {
			t.Fatalf("position %d at (%d,%d): hit %d want %d", pos, x, y, got, composerPosition(m.input))
		}
		top := m.height - m.input.Height() - 2 - m.terminalHeight()
		// Move away on the same viewport, then click the recorded location.
		m.composerSelection = nil
		event := tea.MouseMsg{X: m.contentOffset() + 1 + x, Y: top + y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
		m.Update(event)
		if composerPosition(m.input) != got {
			t.Fatalf("click moved to %d want %d", composerPosition(m.input), got)
		}
		event.Action = tea.MouseActionRelease
		m.Update(event)
	}
}

func TestComposerMouseDragAndReplace(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("one two three")
	m.resize()
	top := m.height - m.input.Height() - 2 - m.terminalHeight()
	left := m.contentOffset() + 3
	m.Update(tea.MouseMsg{X: left + 4, Y: top, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.MouseMsg{X: left + 7, Y: top, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m.Update(tea.MouseMsg{X: left + 7, Y: top, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	if got := m.selectedComposerText(); got != "two" {
		t.Fatalf("drag selected %q", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("changed"), Paste: true})
	if m.input.Value() != "one changed three" {
		t.Fatalf("paste replacement: %q", m.input.Value())
	}
}

func TestComposerSelectionMultilineAndStale(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("first\nsecond")
	m.resize()
	m.setComposerPosition(2)
	m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	if got := m.selectedComposerText(); got != "rst\nse" {
		t.Fatalf("down selection %q", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	if m.selectedComposerText() != "" {
		t.Fatal("reverse selection did not shrink")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	m.input.SetValue("different")
	if m.selectedComposerText() != "" {
		t.Fatal("selection survived external draft replacement")
	}
	m.input.CursorEnd()
	m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	m.sessions[0].Id = "other"
	if m.selectedComposerText() != "" {
		t.Fatal("selection leaked across sessions")
	}
}

func TestComposerSelectionDeletesWholeChip(t *testing.T) {
	m := conversationModel()
	token := "[Paste fixture]"
	m.pastes = map[string]*pastedText{token: {token: token, body: "private body"}}
	m.input.SetValue("a " + token + " z")
	m.setComposerPosition(5)
	m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	if m.selectedComposerText() != token {
		t.Fatalf("partial chip: %q", m.selectedComposerText())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if m.input.Value() != "a  z" {
		t.Fatalf("split chip: %q", m.input.Value())
	}
}

func TestComposerSelectionCopyCutAndEscape(t *testing.T) {
	m := conversationModel()
	var out bytes.Buffer
	m.cursorOutput = &cursorWriter{out: &out}
	m.input.SetValue("한글 copy")
	for range 4 {
		m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !strings.Contains(out.String(), ansi.SetSystemClipboard("copy")) || m.input.Value() != "한글 copy" {
		t.Fatal("copy changed input or copied wrong text")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.selectedComposerText() != "" || m.interruptKey != "" {
		t.Fatal("escape interrupted instead of deselecting")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if m.input.Value() != "한글 opy" {
		t.Fatalf("cut: %q", m.input.Value())
	}
}

func TestComposerSelectionGraphemesAndModal(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("A👩‍💻é")
	m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	if m.selectedComposerText() != "é" {
		t.Fatal("split combining grapheme")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	if m.selectedComposerText() != "👩‍💻é" {
		t.Fatal("split emoji grapheme")
	}
	m.composerSelection = nil
	m.panelFocus = true
	m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	if m.composerSelection != nil {
		t.Fatal("sidebar selected composer")
	}
	m.panelFocus = false
	m.settingsPage = &settingsPage{}
	if m.composerMouse(tea.MouseMsg{X: m.contentOffset() + 3, Y: m.height - m.input.Height() - 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}) {
		t.Fatal("mouse reached composer through settings")
	}
}
