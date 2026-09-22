package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestOtherDraftAndStableRows(t *testing.T) {
	m := questionModel()
	m.syncQuestion()
	d := m.questionDialog
	d.row = len(d.questions[0].Options)
	base := ansi.Strip(m.questionPanel())
	m.questionKey(questionKeyMsg(strings.Repeat("한글", 100)))
	filled := ansi.Strip(m.questionPanel())
	buttonRow := func(view string) int {
		for i, row := range strings.Split(view, "\n") {
			if strings.Contains(row, "[ Next ]") {
				return i
			}
		}
		return -1
	}
	if buttonRow(base) != buttonRow(filled) {
		t.Fatal("Other typing changed height")
	}
	d.row = 0
	m.questionKey(tea.KeyMsg{Type: tea.KeyEnter})
	if d.other[0].Value() == "" || d.otherSelected[0] || d.texts()[0] != "" {
		t.Fatal("draft discarded or inactive Other submitted")
	}
	d.row = len(d.questions[0].Options)
	m.questionKey(tea.KeyMsg{Type: tea.KeyLeft})
	if d.otherSelected[0] || !d.selected[0][0] {
		t.Fatal("cursor navigation changed selection")
	}
	m.questionKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !d.otherSelected[0] || d.selected[0][0] {
		t.Fatal("Other could not be reselected")
	}
}

func TestRecreateConfirmationEditingAndPaste(t *testing.T) {
	m := newRecreateConfirmation()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("recreat"), Paste: true})
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil || m.confirmed || m.message == "" {
		t.Fatal("mismatch confirmed or exited")
	}
	m.Update(questionKeyMsg("e"))
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil || !m.confirmed {
		t.Fatal("cannot edit confirmation")
	}
	m = newRecreateConfirmation()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("recreate"), Paste: true})
	if m.confirmed {
		t.Fatal("paste auto-confirmed")
	}
}
