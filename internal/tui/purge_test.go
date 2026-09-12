package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/internal/purge"
	"strings"
	"testing"
)

func TestPurgeRequiresConfirmButton(t *testing.T) {
	m := newPurgeModel(purge.Plan{Targets: []purge.Target{{Kind: "local", Name: "settings.json", Group: "local"}}})
	for _, g := range purge.Groups {
		if !m.selected[g.ID] {
			t.Fatal("not checked by default")
		}
	}
	if strings.Count(m.View(), "[x]") != len(purge.Groups) || !strings.Contains(m.View(), "Confirm irreversible deletion") {
		t.Fatal("missing checklist/confirm")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.confirmed {
		t.Fatal("Enter on checkbox confirmed")
	}
	if strings.Count(m.View(), "[x]") != len(purge.Groups)-1 {
		t.Fatal("checkbox did not render unchecked")
	}
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.focus = len(purge.Groups)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.confirmed || cmd == nil {
		t.Fatal("confirm button failed")
	}
}
func TestPurgeCancellationAndSmallTerminal(t *testing.T) {
	m := newPurgeModel(purge.Plan{})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil || m.confirmed {
		t.Fatal("cancel failed")
	}
	m.focus = len(purge.Groups)
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.confirmed {
		t.Fatal("confirmed invisible button")
	}
}
