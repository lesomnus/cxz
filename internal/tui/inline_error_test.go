package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestInlineTrustChoiceAndScrolling(t *testing.T) {
	m := &inlineError{message: strings.Repeat("privileged requested\n", 30) + "LAST", trust: true, width: 80, height: 24}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "› [ Exit ]     [ Trust ]") || !strings.Contains(view, "  [ Trust ]") || strings.Contains(view, "?1049") {
		t.Fatal(view)
	}
	if len(strings.Split(view, "\n")) >= 24 {
		t.Fatal("used full screen")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if !strings.Contains(m.View(), "LAST") {
		t.Fatal("cannot inspect full error")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab, Paste: true})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter, Paste: true})
	if m.selected != 0 || m.accepted || m.done {
		t.Fatal("paste selected trust")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.accepted || !m.done {
		t.Fatal("trust failed")
	}
	for _, key := range []tea.KeyType{tea.KeyEnter, tea.KeyEsc, tea.KeyCtrlC} {
		m = &inlineError{trust: true}
		m.Update(tea.KeyMsg{Type: key})
		if m.accepted || !m.done {
			t.Fatal("default did not exit")
		}
	}
}
