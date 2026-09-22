package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

type sessionDeleteConfirmation struct {
	id, run string
	until   time.Time
}

type sessionDeleted struct {
	id  string
	err error
}

func (m *model) updateDeleteConfirmation(msg tea.Msg, now time.Time) {
	if m.deleteConfirm == nil {
		return
	}
	if !now.Before(m.deleteConfirm.until) {
		m.deleteConfirm = nil
		return
	}
	switch v := msg.(type) {
	case tea.KeyMsg:
		if v.Paste || v.String() != "ctrl+x" {
			m.deleteConfirm = nil
		}
	case tea.MouseMsg:
		if v.Action == tea.MouseActionPress || v.Button == tea.MouseButtonWheelUp || v.Button == tea.MouseButtonWheelDown {
			m.deleteConfirm = nil
		}
	case tea.BlurMsg:
		m.deleteConfirm = nil
	}
}

func (m *model) validateDeleteSelection() {
	if d := m.deleteConfirm; d != nil {
		rows := m.panelRows()
		if m.panelIndex < 0 || m.panelIndex >= len(rows) || rows[m.panelIndex].session == nil || rows[m.panelIndex].session.Id != d.id || rows[m.panelIndex].session.RunId != d.run {
			m.deleteConfirm = nil
		}
	}
}

func (m *model) confirmSessionDelete(now time.Time) tea.Cmd {
	if m.deletingID != "" {
		return nil
	}
	rows := m.panelRows()
	if m.panelIndex < 0 || m.panelIndex >= len(rows) || rows[m.panelIndex].session == nil {
		m.deleteConfirm = nil
		m.notice = "Select a session to delete."
		return nil
	}
	s := rows[m.panelIndex].session
	if d := m.deleteConfirm; d == nil || d.id != s.Id || d.run != s.RunId || !now.Before(d.until) {
		m.deleteConfirm = &sessionDeleteConfirmation{id: s.Id, run: s.RunId, until: now.Add(3 * time.Second)}
		return nil
	}
	m.deleteConfirm = nil
	m.deletingID = s.Id
	id, client, parent := s.Id, m.client, m.ctx
	return func() tea.Msg {
		c, ok := client.(interface {
			DeleteSession(context.Context, string) error
		})
		if !ok {
			return sessionDeleted{id: id, err: fmt.Errorf("session deletion unsupported")}
		}
		ctx, cancel := context.WithTimeout(parent, 35*time.Second)
		defer cancel()
		return sessionDeleted{id: id, err: c.DeleteSession(ctx, id)}
	}
}

func (m *model) receiveSessionDeleted(v sessionDeleted) {
	if m.deletingID != v.id {
		return
	}
	m.deletingID = ""
	if v.err != nil {
		m.showError("Session deletion failed: " + v.err.Error())
		return
	}
	currentID := ""
	if current := m.current(); current != nil {
		currentID = current.Id
		if currentID == v.id {
			m.backToProject()
			m.panelFocus = true
			m.input.Blur()
		}
	}
	sessions := m.allSessions
	if sessions == nil {
		sessions = m.sessions
	}
	kept := make([]*api.Session, 0, len(sessions))
	for _, s := range sessions {
		if s.Id != v.id {
			kept = append(kept, s)
		}
	}
	m.updatePanel(listing{sessions: kept})
	m.sessions = ProjectSessions(kept, m.project)
	m.selected = max(0, min(m.selected, len(m.sessions)-1))
	for i, s := range m.sessions {
		if s.Id == currentID {
			m.selected = i
			break
		}
	}
	m.notice = "session deleted; journal retained"
	m.resize()
	m.render()
}
