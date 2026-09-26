package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

type archiveClient struct {
	projectClient
	restored, project string
}

func (c *archiveClient) ArchivedSessions(_ context.Context, project string) ([]*api.Session, error) {
	c.project = project
	return []*api.Session{{Id: "saved-session", ProjectId: project, Agent: "claude", Title: "Saved conversation"}}, nil
}
func (c *archiveClient) RestoreSession(_ context.Context, id string) (*api.Session, error) {
	c.restored = id
	return &api.Session{Id: id, ProjectId: c.project, State: "stopped"}, nil
}
func TestNewSessionScreenRestoresExistingIdentity(t *testing.T) {
	m := projectModel()
	c := &archiveClient{}
	m.client = c
	m.accountView, m.accountChoosing = true, true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if cmd == nil || m.sessionArchive == nil {
		t.Fatal("new-session page did not offer restoration")
	}
	m.Update(cmd())
	if c.project != "p" || len(m.sessionArchive.items) != 1 {
		t.Fatal("archive was not scoped to project")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("restore not submitted")
	}
	m.Update(cmd())
	if c.restored != "saved-session" || m.wantID != "saved-session" || m.sessionArchive != nil || m.accountView {
		t.Fatal("restore created or opened the wrong session")
	}
}
