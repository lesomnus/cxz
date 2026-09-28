package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

const sessionNavigationLimit = 100

type sessionNavigation struct {
	ids    []string
	index  int
	moving bool
}

func (m *model) visibleSessionID() string {
	if m.projectView || m.accountView || m.settingsPage != nil || m.memoryPage != nil {
		return ""
	}
	if s := m.current(); s != nil {
		return s.Id
	}
	return ""
}
func (n *sessionNavigation) visit(id string) {
	if id == "" || len(n.ids) > 0 && n.ids[n.index] == id {
		return
	}
	if len(n.ids) > 0 {
		n.ids = n.ids[:n.index+1]
	}
	n.ids = append(n.ids, id)
	if len(n.ids) > sessionNavigationLimit {
		n.ids = append([]string(nil), n.ids[len(n.ids)-sessionNavigationLimit:]...)
	}
	n.index = len(n.ids) - 1
}
func (m *model) observeSessionNavigation() func() {
	before := m.visibleSessionID()
	if len(m.sessionNavigation.ids) == 0 {
		m.sessionNavigation.visit(before)
	}
	return func() {
		n := &m.sessionNavigation
		if n.moving {
			n.moving = false
			return
		}
		after := m.visibleSessionID()
		if after != "" && after != before {
			n.visit(before)
			n.visit(after)
		}
	}
}
func (m *model) navigationSession(id string) *api.Session {
	sessions := m.allSessions
	if sessions == nil {
		sessions = m.sessions
	}
	for _, s := range sessions {
		if s.Id == id {
			return s
		}
	}
	return nil
}
func (m *model) navigateSession(direction int) tea.Cmd {
	n := &m.sessionNavigation
	for index := n.index + direction; index >= 0 && index < len(n.ids); index += direction {
		s := m.navigationSession(n.ids[index])
		if s == nil {
			continue
		}
		p := &api.Project{Id: s.ProjectId, Name: s.ProjectName, Alias: s.ProjectAlias, Workspace: s.Workspace}
		for _, candidate := range m.panelProjects {
			if candidate.Id == s.ProjectId {
				p = candidate
				break
			}
		}
		n.index = index
		n.moving = true
		m.selectPanelProject(panelRow{project: p, session: s})
		m.settingsPage = nil
		if p := m.memoryPage; p != nil && p.cancel != nil {
			p.cancel()
		}
		m.memoryPage = nil
		m.filePreview = nil
		m.toolSelector = nil
		m.textSelection = nil
		m.panelFocus = false
		m.projectView = false
		m.restoreDraft()
		m.view.GotoBottom()
		m.panelWantKey = "s:" + s.Id
		for i, row := range m.panelRows() {
			if row.session != nil && row.session.Id == s.Id {
				m.panelIndex = i
				break
			}
		}
		m.watch()
		m.resize()
		m.render()
		return m.input.Focus()
	}
	return nil
}
func (m *model) sessionNavigationKey(k tea.KeyMsg) (bool, tea.Cmd) {
	if k.Paste || (k.String() != "alt+left" && k.String() != "alt+right") {
		return false, nil
	}
	// Do not discard in-progress forms or steal shell editing shortcuts.
	if m.renaming || m.settingsPage != nil && (m.settingsPage.confirm != "" || m.settingsPage.busy) || m.memoryPage != nil && m.memoryPage.copy != nil || m.terminalFocused() || m.workflow != nil || m.creating || m.accountView || m.pasteDialog != nil || m.redactDialog != nil || m.restartConfirm != nil || m.sessionArchive != nil || m.errorFocused() {
		return false, nil
	}
	direction := -1
	if k.String() == "alt+right" {
		direction = 1
	}
	return true, m.navigateSession(direction)
}
