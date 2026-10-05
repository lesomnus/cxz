package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"strings"
	"testing"
)

type projectTitleClient struct {
	recordingClient
	id, title string
}

func (c *projectTitleClient) RenameProject(_ context.Context, id, title string) error {
	c.id, c.title = id, title
	return nil
}
func TestF2RenamesSelectedProjectTitleAndPreservesDraft(t *testing.T) {
	m := panelModel()
	c := &projectTitleClient{}
	m.client = c
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	m.input.SetValue("keep draft")
	m.panelFocus = true
	m.panelIndex = 1 // Other project's session, not the current conversation.
	rows := m.panelRows()
	id := rows[1].project.Id
	alias := rows[1].project.Alias
	m.Update(tea.KeyMsg{Type: tea.KeyF2})
	if !m.renaming || !m.renameProject || m.renameID != id {
		t.Fatal("wrong target", m.renameID)
	}
	m.aliasInput.SetValue("A new title")
	if !strings.Contains(ansi.Strip(m.panelScreen()), "A new title") {
		t.Fatal("editor not visible")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("missing save")
	}
	m.Update(cmd())
	if c.id != id || c.title != "A new title" || m.renaming || m.input.Value() != "keep draft" {
		t.Fatal("save/draft", c, m.input.Value())
	}
	for _, p := range m.panelProjects {
		if p.Id == id && (p.Name != "A new title" || p.Alias != alias) {
			t.Fatal("changed alias or lost title")
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m.aliasInput.SetValue("discard")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.renaming || c.title != "A new title" {
		t.Fatal("cancel saved")
	}
}
func TestF2F3NeverAnswerApproval(t *testing.T) {
	for _, focused := range []bool{false, true} {
		m := panelModel()
		c := &projectTitleClient{}
		m.client = c
		m.panelFocus = false
		m.projectView = false
		m.focusApproval = focused
		m.current().Pending = []*api.Event{{RunId: m.current().RunId, RequestId: "permission", Text: "Bash"}}
		m.Update(tea.KeyMsg{Type: tea.KeyF3})
		m.Update(tea.KeyMsg{Type: tea.KeyF2})
		if len(c.answers) != 0 || !m.renaming || m.renameID != m.current().ProjectId {
			t.Fatal("wrong F2/F3 action")
		}
	}
}

func TestF2F3DoNotDecideMCPConsent(t *testing.T) {
	m := confirmationModel("ask")
	for _, key := range []tea.KeyType{tea.KeyF2, tea.KeyF3} {
		handled, cmd := m.elicitationKey(m.current().Pending[0], tea.KeyMsg{Type: key})
		if handled || cmd != nil {
			t.Fatal("legacy MCP shortcut still active")
		}
	}
}
