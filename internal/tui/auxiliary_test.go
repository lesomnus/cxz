package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"testing"
)

func TestAuxiliarySuggestionRequiresCurrentSourceAndEmptyDraft(t *testing.T) {
	m := conversationModel()
	connection := m.connectionRef()
	j := &auxiliary.Job{ID: "job", Session: "s", Run: "run", Turn: 3, Status: "completed", Suggestion: "다음 작업을 진행해줘"}
	receive := func() {
		m.receiveAuxiliary(auxiliaryResult{connection: connection, session: "s", action: "apply", reply: auxiliary.Reply{Job: j}})
	}
	m.input.SetValue("my draft")
	receive()
	if m.input.Value() != "my draft" {
		t.Fatal("overwrote draft")
	}
	m.input.Reset()
	m.events["s"] = []*api.Event{{Kind: "input", Seq: 4}}
	receive()
	if m.input.Value() != "" {
		t.Fatal("applied stale suggestion")
	}
	m.events["s"] = nil
	j.Run = "old"
	receive()
	if m.input.Value() != "" {
		t.Fatal("applied old run")
	}
	j.Run = "run"
	receive()
	if m.input.Value() != j.Suggestion {
		t.Fatal("current suggestion not applied")
	}
}
func TestAuxiliaryResponsesCannotCrossConnectionOrPage(t *testing.T) {
	m := conversationModel()
	m.receiveAuxiliary(auxiliaryResult{connection: "other", session: "s", reply: auxiliary.Reply{Job: &auxiliary.Job{Summary: "foreign"}}})
	if m.auxiliaryJob() != nil {
		t.Fatal("foreign result accepted")
	}
	p := &auxiliaryPage{}
	m.settingsPage = &settingsPage{auxiliary: p}
	m.receiveAuxiliary(auxiliaryResult{page: &auxiliaryPage{}, reply: auxiliary.Reply{Message: "old"}})
	if p.message != "" {
		t.Fatal("old page changed current")
	}
}
func TestAltEnterStillInsertsNewline(t *testing.T) {
	m := conversationModel()
	m.input.Focus()
	m.input.SetValue("draft")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	if m.input.Value() != "draft\n" {
		t.Fatal(m.input.Value())
	}
}
