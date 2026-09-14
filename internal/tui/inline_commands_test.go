package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

func TestInlineRedactPreservesDraftAndCursor(t *testing.T) {
	for _, draft := range []string{"foo @redact bar", "첫 줄\n비밀 @redact 다음 줄\n끝"} {
		m := conversationModel()
		pos := len([]rune(strings.Split(draft, "@redact")[0])) + len("@redact")
		m.setPathInput(draft, pos)
		if _, hints := m.inlineHints(); len(hints) != 1 {
			t.Fatal("missing inline hint")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.redactDialog == nil || m.input.Value() != draft {
			t.Fatal("modal changed draft")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("private"), Paste: true})
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		want := strings.Replace(draft, "@redact", "[Redacted]", 1)
		if m.input.Value() != want {
			t.Fatalf("got %q want %q", m.input.Value(), want)
		}
		_, cursor, _, _, _ := m.chipInput()
		if cursor != pos-len("@redact")+len("[Redacted]") {
			t.Fatal("cursor not after chip", cursor)
		}
	}
}
func TestInlineRedactCancelAndCompletion(t *testing.T) {
	m := conversationModel()
	m.setPathInput("foo @red bar", 8)
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.input.Value() != "foo @redact bar" || m.redactDialog != nil {
		t.Fatal("completion changed surrounding text")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.input.Value() != "foo @redact bar" || m.redactDialog != nil {
		t.Fatal("cancel lost draft")
	}
	_, pos, _, _, _ := m.chipInput()
	if pos != 11 {
		t.Fatal("cancel lost cursor", pos)
	}
	if _, hints := m.inlineHints(); len(hints) != 0 {
		t.Fatal("cancel immediately reopened hints")
	}
}
func TestInlineRedactTriggerBoundaries(t *testing.T) {
	for _, input := range []string{"@", "foo @", "foo @red", "\n@redact"} {
		m := conversationModel()
		m.input.SetValue(input)
		if _, hints := m.inlineHints(); len(hints) != 1 {
			t.Fatal("missing trigger", input)
		}
	}
	for _, input := range []string{"user@example.com", "foo@redact", "` @redact", "foo @unknown"} {
		m := conversationModel()
		m.input.SetValue(input)
		if _, hints := m.inlineHints(); len(hints) != 0 {
			t.Fatal("false trigger", input)
		}
	}
	m := conversationModel()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("foo @redact"), Paste: true})
	if _, hints := m.inlineHints(); len(hints) != 0 {
		t.Fatal("paste activated command")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.redactDialog != nil || m.input.Value() != "foo @redact\n" {
		t.Fatal("paste Enter executed command")
	}
	for _, c := range slashCommands {
		if c.name == "/redact" {
			t.Fatal("old slash command advertised")
		}
	}
}
