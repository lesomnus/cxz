package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

type projectClient struct {
	api.SessionsClient
	deleted []string
}

func (c *projectClient) DeleteSession(_ context.Context, id string) error {
	c.deleted = append(c.deleted, id)
	return nil
}
func projectModel() *model {
	return &model{ctx: context.Background(), client: &projectClient{}, project: &api.Project{Id: "p", Name: "Project", Workspace: "/work", State: "running"}, projectView: true, input: textarea.New(), view: viewport.New(80, 15), width: 100, height: 25, events: map[string][]*api.Event{}, cursor: map[string]uint64{}}
}
func TestProjectScopeAndLatest(t *testing.T) {
	m := projectModel()
	input := []*api.Session{{Id: "old", ProjectId: "p", CreatedAt: 1}, {Id: "foreign", ProjectId: "other", CreatedAt: 9}, {Id: "new", ProjectId: "p", CreatedAt: 2}}
	m.Update(listing{sessions: input})
	if len(m.sessions) != 2 || m.current().Id != "new" || strings.Contains(m.View(), "foreign") {
		t.Fatal("wrong project/latest selection")
	}
	if input[0].Id != "old" {
		t.Fatal("mutated caller inventory")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.projectView || m.current().Id != "old" {
		t.Fatal("did not open selected session")
	}
	canceled := false
	m.watchCancel = func() { canceled = true }
	m.watchID = "old"
	m.input.SetValue("unsent")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if !m.projectView || !canceled || m.watchID != "" || m.input.Value() != "" {
		t.Fatal("did not return to project safely")
	}
}
func TestProjectDeleteConfirmation(t *testing.T) {
	m := projectModel()
	m.sessions = []*api.Session{{Id: "target", ProjectId: "p"}, {Id: "other", ProjectId: "p"}}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.deletingID != "target" {
		t.Fatal("missing confirmation")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.client.(*projectClient).deleted) != 0 {
		t.Fatal("deleted on cancel")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m.selected = 1
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("missing delete")
	}
	cmd()
	if got := m.client.(*projectClient).deleted; len(got) != 1 || got[0] != "target" {
		t.Fatal("deleted changed selection", got)
	}
}
func TestEmptyProjectAndCreatedSession(t *testing.T) {
	m := projectModel()
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.projectView || !strings.Contains(m.View(), "No sessions") {
		t.Fatal("empty project entered session")
	}
	m.Update(result{sessionID: "created"})
	m.Update(listing{sessions: []*api.Session{{Id: "created", ProjectId: "p"}}})
	if m.projectView || m.current().Id != "created" {
		t.Fatal("new session not selected")
	}
}
