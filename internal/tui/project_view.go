package tui

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

type ProjectCreator func(context.Context, string, string, io.Reader, io.Writer, io.Writer) (*api.Session, error)
type AccountLogin func(context.Context, string, string, string, io.Reader, io.Writer, io.Writer) error

func ProjectSessions(sessions []*api.Session, p *api.Project) []*api.Session {
	var out []*api.Session
	for _, s := range sessions {
		if p == nil || s.ProjectId == p.Id {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt == out[j].CreatedAt {
			return out[i].Id > out[j].Id
		}
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out
}

func (m *model) backToProject() {
	if m.project == nil {
		if s := m.current(); s != nil {
			m.project = &api.Project{Id: s.ProjectId, Name: s.ProjectName, Alias: s.ProjectAlias, Workspace: s.Workspace}
		}
	}
	if m.project == nil {
		return
	}
	m.saveDraft()
	m.projectView = true
	m.sessions = ProjectSessions(m.sessions, m.project)
	m.selected = max(0, min(m.selected, len(m.sessions)-1))
	m.creating = false
	m.focusList = false
	m.focusApproval = false
	m.interruptKey = ""
	m.approvalOffset = 0
	m.deletingID = ""
	m.wantID = ""
	m.input.Reset()
	if m.watchCancel != nil {
		m.watchCancel()
		m.watchCancel = nil
	}
	m.watchID = ""
}

func (m *model) projectAction(key tea.KeyMsg) tea.Cmd {
	if key.String() == "ctrl+d" {
		return tea.Quit
	}
	if m.busy {
		return nil
	}
	if m.deletingID != "" {
		if key.String() == "esc" || key.String() == "n" {
			m.deletingID = ""
			m.notice = "deletion canceled"
			return nil
		}
		if key.String() != "y" {
			return nil
		}
		id := m.deletingID
		m.deletingID = ""
		m.busy = true
		return func() tea.Msg {
			c, ok := m.client.(interface {
				DeleteSession(context.Context, string) error
			})
			if !ok {
				return result{err: fmt.Errorf("session deletion unsupported")}
			}
			ctx, cancel := context.WithTimeout(m.ctx, 35*time.Second)
			defer cancel()
			err := c.DeleteSession(ctx, id)
			return result{text: "session deleted; journal retained", err: err}
		}
	}
	switch key.String() {
	case "a":
		return m.openAccounts(false)
	case "ctrl+n", "n":
		if m.project != nil && m.project.State != "connection" {
			return m.openAccounts(true)
		}
		m.creationConnection = m.connectionRef()
		m.creating = true
		m.input.SetValue(m.project.Workspace)
		if m.project.Workspace == "" {
			m.input.Placeholder = "absolute workspace path; Enter creates, Esc cancels"
		}
		m.notice = "Loading accounts…"
		return tea.Batch(m.input.Focus(), m.loadAccounts())

	}
	return nil
}
