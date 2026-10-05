package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestComposerLogicalGutterAndWhitespace(t *testing.T) {
	m := conversationModel()
	m.width = 22
	value := "한글 hello words wrap onto another row\nnext line\n\nlast"
	m.input.SetValue(value)
	m.resize()
	m.showInputWhitespace = true
	for pos := 0; pos <= utf8.RuneCountInString(value); pos++ {
		m.setComposerPosition(pos)
		m.resize()
		layout := m.composerRows()
		offset := m.composerScroll(layout)
		before := m.input.View()
		after := m.composerDisplay(before)
		rawRows := strings.Split(before, "\n")
		rows := strings.Split(ansi.Strip(after), "\n")
		for y, text := range rows {
			if ansi.StringWidth(text) != ansi.StringWidth(rawRows[y]) {
				t.Fatal("changed display width")
			}
			if offset+y >= len(layout) {
				continue
			}
			row := layout[offset+y]
			gutter := ansi.Cut(text, 0, 2)
			if row.column > 0 && gutter != "  " {
				t.Fatalf("soft wrap has number: %q", text)
			}
			if row.column == 0 && row.line == 1 && gutter != "1 " {
				t.Fatalf("logical second line: %q", text)
			}
			runes := []rune(value)
			hasNewline := row.end < len(runes) && runes[row.end] == '\n'
			if strings.Contains(text, "↵") != hasNewline {
				t.Fatalf("newline mismatch: %q row=%+v", text, row)
			}
		}
		if m.input.Value() != value || composerPosition(m.input) != pos {
			t.Fatal("render mutated input")
		}
	}
}

func TestComposerWhitespaceTogglePreservesSelectionAndChips(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("a b\n[Paste 1] ")
	m.pastes = map[string]*pastedText{"[Paste 1]": {body: "secretly many lines"}}
	m.resize()
	m.setComposerPosition(3)
	m.extendComposerSelection(func() { m.setComposerPosition(0) })
	selected := m.selectedComposerText()
	key := tea.KeyMsg{Type: tea.KeyRunes, Alt: true, Runes: []rune("w")}
	handled, _ := m.composerKey(key)
	if !handled || !m.showInputWhitespace || m.selectedComposerText() != selected {
		t.Fatal("toggle lost selection")
	}
	view := ansi.Strip(m.composerDisplay(m.input.View()))
	if !strings.Contains(view, "a·b↵") || !strings.Contains(view, "[Paste 1]·") {
		t.Fatalf("visible whitespace/chip: %q", view)
	}
	m.composerKey(key)
	if m.showInputWhitespace || strings.Contains(ansi.Strip(m.composerDisplay(m.input.View())), "·") {
		t.Fatal("toggle off")
	}
	key.Paste = true
	m.composerKey(key)
	if m.showInputWhitespace {
		t.Fatal("pasted shortcut toggled")
	}
}

func TestComposerCellReplacementPreservesCursorSGR(t *testing.T) {
	input := "❯ a\x1b[7m \x1b[27m한글 "
	got := replaceComposerCells(input, map[int]string{3: "·", 8: "↵"})
	if got != "❯ a\x1b[7m·\x1b[27m한글↵" {
		t.Fatalf("lost cursor/style/cell alignment: %q", got)
	}
}
