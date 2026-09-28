package tui

import (
	"fmt"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

func navigationModel() *model {
	m := conversationModel()
	m.sessionNavigation = sessionNavigation{}
	a := m.current()
	a.Id = "a"
	a.ProjectId = "p1"
	m.allSessions = []*api.Session{a, {Id: "b", ProjectId: "p2", RunId: "rb", Agent: "claude"}, {Id: "c", ProjectId: "p1", RunId: "rc", Agent: "claude"}}
	m.sessions = []*api.Session{a}
	m.project = &api.Project{Id: "p1"}
	m.panelProjects = []*api.Project{{Id: "p1"}, {Id: "p2"}}
	return m
}
func visitSession(t *testing.T, m *model, id string) {
	t.Helper()
	for i, row := range m.panelRows() {
		if row.session != nil && row.session.Id == id {
			m.panelIndex = i
			m.panelFocus = true
			m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if m.current() == nil || m.current().Id != id {
				t.Fatal("selection failed", id)
			}
			return
		}
	}
	t.Fatal("missing session", id)
}
func historyKey(m *model, direction int) {
	k := tea.KeyLeft
	if direction > 0 {
		k = tea.KeyRight
	}
	m.Update(tea.KeyMsg{Type: k, Alt: true})
}
func TestSessionNavigationBackForwardDraftsAndBranches(t *testing.T) {
	m := navigationModel()
	m.input.SetValue("draft a")
	visitSession(t, m, "b")
	m.input.SetValue("draft b")
	visitSession(t, m, "c")
	if !reflect.DeepEqual(m.sessionNavigation.ids, []string{"a", "b", "c"}) {
		t.Fatal(m.sessionNavigation)
	}
	historyKey(m, -1)
	if m.current().Id != "b" || m.project.Id != "p2" || m.input.Value() != "draft b" {
		t.Fatal("back did not restore session/draft")
	}
	historyKey(m, -1)
	if m.current().Id != "a" || m.input.Value() != "draft a" {
		t.Fatal("second back")
	}
	historyKey(m, -1)
	if m.current().Id != "a" {
		t.Fatal("wrapped at start")
	}
	historyKey(m, 1)
	if m.current().Id != "b" {
		t.Fatal("forward")
	}
	visitSession(t, m, "a")
	if !reflect.DeepEqual(m.sessionNavigation.ids, []string{"a", "b", "a"}) {
		t.Fatal("forward tail retained", m.sessionNavigation)
	}
	historyKey(m, 1)
	if m.current().Id != "a" {
		t.Fatal("forward after branching")
	}
}
func TestSessionNavigationExcludesOtherViewsAndDuplicates(t *testing.T) {
	m := navigationModel()
	visitSession(t, m, "b")
	visitSession(t, m, "b")
	m.Update(tea.KeyMsg{Type: tea.KeyF19})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.openReport("/context", "report")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !reflect.DeepEqual(m.sessionNavigation.ids, []string{"a", "b"}) {
		t.Fatal("non-session or duplicate recorded", m.sessionNavigation)
	}
	historyKey(m, -1)
	if m.current().Id != "a" || m.report != nil || m.settingsPage != nil {
		t.Fatal("back failed")
	}
}
func TestSessionNavigationSkipsDeletedSessions(t *testing.T) {
	m := navigationModel()
	visitSession(t, m, "b")
	visitSession(t, m, "c")
	m.allSessions = []*api.Session{m.allSessions[0], m.allSessions[2]}
	historyKey(m, -1)
	if m.current().Id != "a" {
		t.Fatal("deleted session was not skipped")
	}
	historyKey(m, 1)
	if m.current().Id != "c" {
		t.Fatal("forward skip")
	}
}
func TestSessionNavigationLimitAndPaste(t *testing.T) {
	var n sessionNavigation
	for i := 0; i < sessionNavigationLimit+10; i++ {
		n.visit(fmt.Sprint(i))
	}
	if len(n.ids) != sessionNavigationLimit || n.ids[0] != "10" || n.index != sessionNavigationLimit-1 {
		t.Fatal(n)
	}
	m := navigationModel()
	visitSession(t, m, "b")
	if handled, _ := m.sessionNavigationKey(tea.KeyMsg{Type: tea.KeyLeft, Alt: true, Paste: true}); handled {
		t.Fatal("paste became navigation")
	}
	if m.current().Id != "b" {
		t.Fatal("paste switched session")
	}
}
func TestSessionNavigationFromSettingsOnlyVisitsSessions(t *testing.T) {
	m := navigationModel()
	visitSession(t, m, "b")
	m.Update(tea.KeyMsg{Type: tea.KeyF19})
	if m.settingsPage == nil {
		t.Fatal("settings did not open")
	}
	historyKey(m, -1)
	if m.settingsPage != nil || m.current().Id != "a" {
		t.Fatal("settings became a history stop")
	}
	if !reflect.DeepEqual(m.sessionNavigation.ids, []string{"a", "b"}) {
		t.Fatal(m.sessionNavigation)
	}
}

func TestSessionNavigationPreservesActiveForms(t *testing.T) {
	m := navigationModel()
	visitSession(t, m, "b")
	m.renaming = true
	historyKey(m, -1)
	if m.current().Id != "b" || !m.renaming {
		t.Fatal("navigation discarded rename form")
	}
	m.renaming = false
	m.settingsPage = &settingsPage{confirm: "history-policy"}
	historyKey(m, -1)
	if m.current().Id != "b" || m.settingsPage == nil {
		t.Fatal("navigation discarded confirmation")
	}
}
