package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

func TestComposerLineEditingAndDetach(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("a\nb")
	m.setComposerPosition(0)
	m.Update(tea.KeyMsg{Type: tea.KeyDown, Alt: true})
	if m.input.Value() != "b\na" {
		t.Fatal(m.input.Value())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}, Alt: true})
	if m.input.Value() != "b\na\na" {
		t.Fatal(m.input.Value())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if m.input.Value() != "b\na" {
		t.Fatal("undo", m.input.Value())
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+D no longer detaches")
	}
}
func TestComposerIndentAndNewline(t *testing.T) {
	for _, k := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyEnter, Alt: true}, {Type: tea.KeyCtrlJ}} {
		m := conversationModel()
		m.input.SetValue("  a\nb")
		m.setComposerPosition(0)
		m.extendComposerSelection(func() { m.setComposerPosition(5) })
		m.Update(tea.KeyMsg{Type: tea.KeyTab})
		if m.input.Value() != "      a\n    b" {
			t.Fatal(m.input.Value())
		}
		m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		if m.input.Value() != "  a\nb" {
			t.Fatal(m.input.Value())
		}
		m.input.ClearSelection()
		m.setComposerPosition(3)
		m.Update(k)
		if m.input.Value() != "  a\n  \nb" {
			t.Fatal(k.String(), m.input.Value())
		}
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
		if m.input.Value() != "  a\nb" {
			t.Fatal("newline undo", m.input.Value())
		}
	}
}
func TestComposerMultiClickAndScrolledRendering(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("hello world\nnext")
	m.setComposerPosition(0)
	m.resize()
	click := func() {
		x, y := m.contentOffset()+4, m.height-m.input.Height()-2-m.terminalHeight()
		m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	}
	click()
	click()
	if m.selectedComposerText() != "hello" {
		t.Fatal(m.selectedComposerText())
	}
	click()
	if m.selectedComposerText() != "hello world\n" {
		t.Fatal(m.selectedComposerText())
	}
	m.input.SetValue("FIRST\n" + strings.Repeat("middle\n", 30) + "LAST")
	m.setComposerPosition(0)
	m.extendComposerSelection(func() { m.setComposerPosition(2) })
	m.resize()
	for range 20 {
		m.scrollComposer(true)
	}
	if composerPosition(m.input) != 2 || m.selectedComposerText() != "FI" {
		t.Fatal("wheel changed selection")
	}
	if view := m.sessionScreen(); !strings.Contains(view, "LAST") || strings.Contains(view, "FIRST") {
		t.Fatal("render ignored detached viewport")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if !strings.HasPrefix(m.input.Value(), "xRST") || m.composerScroll(m.composerRows()) != 0 {
		t.Fatal("typing did not return to selection")
	}
}

func TestComposerLineEditingKeepsPastePayload(t *testing.T) {
	m := conversationModel()
	body := "first\nsecond\nthird\nlast"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(body), Paste: true})
	token := m.input.Value()
	m.input.SetValue(token + "\nend")
	m.setComposerPosition(0)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}, Alt: true})
	if got := expandPastes(m.input.Value(), m.pastes); got != body+"\n"+body+"\nend" {
		t.Fatal(got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if m.input.Value() != token+"\nend" {
		t.Fatal("chip undo", m.input.Value())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown, Alt: true})
	if got := expandPastes(m.input.Value(), m.pastes); got != "end\n"+body {
		t.Fatal(got)
	}
}

func TestComposerIndentCannotSplitPasteChip(t *testing.T) {
	m := conversationModel()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("one\ntwo\nthree\nfour"), Paste: true})
	token := m.input.Value()
	m.setComposerPosition(4)
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.input.Value() != token {
		t.Fatal("indent split chip", m.input.Value())
	}
}
