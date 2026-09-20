package tui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/lesomnus/cxz/internal/core"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

const maxViewWidth = 133
const projectPanelWidth = 36
const projectPanelGap = 2

type panelRow struct {
	project *api.Project
	session *api.Session
}

func (r panelRow) key() string {
	if r.session != nil {
		return "s:" + r.session.Id
	}
	return "p:" + r.project.Id
}

func (m *model) initializeNavigation(project *api.Project, id string) {
	m.project = project
	m.projectView = id == ""
	m.panelFocus = m.projectView
	if m.panelFocus {
		m.input.Blur()
	}
	if project != nil {
		m.panelProjects = []*api.Project{project}
		if id == "" {
			m.panelWantKey = "p:" + project.Id
		}
	}
}

func (m *model) panelVisible() bool {
	return m.terminalWidth >= maxViewWidth+projectPanelWidth+projectPanelGap && m.height >= 14
}
func (m *model) contentOffset() int {
	if m.panelVisible() {
		return projectPanelWidth + projectPanelGap
	}
	return 0
}

func (m *model) panelRows() []panelRow {
	projects := append([]*api.Project(nil), m.panelProjects...)
	sessions := m.allSessions
	if sessions == nil {
		sessions = m.sessions
	}
	sort.SliceStable(projects, func(i, j int) bool {
		a, b := projects[i], projects[j]
		if a.Name == b.Name {
			return a.Id < b.Id
		}
		return a.Name < b.Name
	})
	var rows []panelRow
	for _, p := range projects {
		rows = append(rows, panelRow{project: p})
		for _, s := range ProjectSessions(sessions, p) {
			rows = append(rows, panelRow{project: p, session: s})
		}
	}
	return rows
}

func (m *model) updatePanel(v listing) {
	rows := m.panelRows()
	key := ""
	if m.panelIndex >= 0 && m.panelIndex < len(rows) {
		key = rows[m.panelIndex].key()
	}
	m.allSessions = v.sessions
	if v.projectsErr != nil {
		m.panelError = "Project list unavailable"
	} else if v.projectsLoaded || v.projects != nil {
		m.panelProjects = v.projects
		m.panelError = ""
	}
	rows = m.panelRows()
	if m.panelWantKey != "" {
		key = m.panelWantKey
	}
	m.panelIndex = max(0, min(m.panelIndex, len(rows)-1))
	for i, r := range rows {
		if r.key() == key {
			m.panelIndex = i
			m.panelWantKey = ""
			break
		}
	}
}

func (m *model) focusPanel() {
	m.panelFocus = true
	m.input.Blur()
	m.pasteSelection = nil
	if s := m.current(); s != nil {
		for i, r := range m.panelRows() {
			if r.session != nil && r.session.Id == s.Id {
				m.panelIndex = i
				break
			}
		}
	}
}

func (m *model) panelKey(k tea.KeyMsg) tea.Cmd {
	if k.Paste {
		return nil
	}
	if k.String() == "ctrl+c" {
		return tea.Quit
	}
	if m.busy {
		return nil
	}
	if m.deletingID != "" {
		return m.projectAction(k)
	}
	rows := m.panelRows()
	switch k.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc", "ctrl+q":
		if m.projectView {
			m.panelFocus = true
			return nil
		}
		m.panelFocus = false
		if !m.projectView && !m.accountView {
			return m.input.Focus()
		}
	case "up":
		m.panelIndex = max(0, m.panelIndex-1)
	case "down":
		m.panelIndex = min(max(0, len(rows)-1), m.panelIndex+1)
	case "home":
		m.panelIndex = 0
	case "end":
		m.panelIndex = max(0, len(rows)-1)
	case "pgup":
		m.panelIndex = max(0, m.panelIndex-max(1, m.height-9))
	case "pgdown":
		m.panelIndex = min(max(0, len(rows)-1), m.panelIndex+max(1, m.height-9))
	case "left":
		for m.panelIndex > 0 && rows[m.panelIndex].session != nil {
			m.panelIndex--
		}
	case "right":
		if m.panelIndex+1 < len(rows) && rows[m.panelIndex].session == nil && rows[m.panelIndex+1].session != nil {
			m.panelIndex++
		}
	case "n", "ctrl+n", "a":
		if len(rows) == 0 {
			m.notice = "Select a project first."
			return nil
		}
		r := rows[m.panelIndex]
		m.selectPanelProject(r)
		m.panelFocus = false
		return m.projectAction(k)
	case "s":
		if len(rows) == 0 || rows[m.panelIndex].session == nil {
			m.notice = "Select a session to stop."
			return nil
		}
		target := rows[m.panelIndex].session
		id, run := target.Id, target.RunId
		m.busy = true
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(m.ctx, 35*time.Second)
			defer cancel()
			_, err := m.client.Stop(ctx, &api.Control{SessionId: id, RunId: run, ClientId: core.ID()})
			return result{text: "session stopped", err: err}
		}
	case "d", "delete":
		if len(rows) == 0 || rows[m.panelIndex].session == nil {
			m.notice = "Select a session to delete."
			return nil
		}
		m.deletingID = rows[m.panelIndex].session.Id
	case "enter":
		if len(rows) == 0 {
			return nil
		}
		r := rows[m.panelIndex]
		if r.session == nil {
			if m.panelIndex+1 < len(rows) && rows[m.panelIndex+1].project.Id == r.project.Id && rows[m.panelIndex+1].session != nil {
				m.panelIndex++
			} else {
				m.notice = "No sessions yet. Press n to create one."
			}
			return nil
		}
		m.selectPanelProject(r)
		m.panelFocus = false
		m.projectView = false
		m.restoreDraft()
		m.view.GotoBottom()
		m.watch()
		m.resize()
		m.render()
		return tea.Batch(m.input.Focus(), m.refresh())
	}
	return nil
}

// Select the command target independently of the original cxz up directory.
func (m *model) selectPanelProject(r panelRow) {
	m.backToProject()
	m.project = r.project
	sessions := m.allSessions
	if sessions == nil {
		sessions = m.sessions
	}
	m.sessions = ProjectSessions(sessions, r.project)
	m.selected = 0
	if r.session != nil {
		for i, s := range m.sessions {
			if s.Id == r.session.Id {
				m.selected = i
				break
			}
		}
	}
	m.questionDialog = nil
	m.pasteDialog = nil
	m.modelPicker = nil
	m.report = nil
	m.restartConfirm = nil
	m.accountView = false
}

func (m *model) panelScreen() string {
	width := projectPanelWidth
	if !m.panelVisible() {
		width = m.width
		if m.terminalWidth > 0 {
			width = min(maxViewWidth, m.terminalWidth)
		}
	}
	rows := m.panelRows()
	capacity := max(1, m.height-9)
	start := max(0, min(m.panelIndex-capacity+3, len(rows)-capacity))
	lines := []string{accent.Bold(true).Render("Projects"), muted.Render("↑/↓ select · Enter open"), ""}
	if len(rows) == 0 {
		lines = append(lines, "No projects")
	}
	for i := start; i < min(len(rows), start+capacity); i++ {
		r := rows[i]
		label := r.project.Name
		if label == "" {
			label = r.project.Alias
		}
		if label == "" {
			label = r.project.Id
		}
		if r.project.Alias != "" && r.project.Alias != label {
			label += " · " + r.project.Alias
		}
		line := lavender.Render(clip(pickerLabel(label), max(1, width-4)))
		if s := r.session; s != nil {
			name := s.Alias
			if name == "" {
				name = s.Id
			}
			line = "  " + pickerLabel(name) + " · " + providerLabel(s.Agent) + " · " + pickerLabel(s.State)
		}
		prefix := "  "
		if current := m.current(); current != nil && !m.projectView && r.session != nil && current.Id == r.session.Id {
			prefix = "• "
		}
		if i == m.panelIndex && m.panelFocus {
			prefix = "› "
			line = accent.Bold(true).Render(ansi.Strip(line))
		}
		lines = append(lines, prefix+line)
	}
	for len(lines) < m.height-6 {
		lines = append(lines, "")
	}
	status := m.notice
	if m.busy {
		status = "Working…"
	}
	if m.deletingID != "" {
		status = fmt.Sprintf("Delete %.8s? y/N · stops agent; journal retained", m.deletingID)
	}
	if m.panelError != "" {
		status = m.panelError
	}
	lines = append(lines, muted.Render("n new · a accounts"), muted.Render("s stop · d delete"), muted.Render("Esc/Ctrl+Q return · Ctrl+C detach"), muted.Render("Agents keep running"), warning.Render(pickerLabel(status)))
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	if m.panelVisible() && m.panelFocus {
		lines[m.height-1] = accent.Render(strings.Repeat("─", max(0, width)))
	}
	background := lipgloss.NewStyle().Background(lipgloss.Color("#10202b"))
	for i := range lines {
		line := clip(lines[i], width)
		lines[i] = background.Render(line + strings.Repeat(" ", max(0, width-ansi.StringWidth(line))))
	}
	return screen(strings.Join(lines, "\n"), width, m.height)
}

func (m *model) wideScreen(content string) string {
	if m.terminalWidth <= 0 {
		return content
	}
	left := []string{}
	if m.panelVisible() {
		left = strings.Split(m.panelScreen(), "\n")
	}
	right := []string{}
	if width := m.previewSideWidth(); width > 0 && !((m.panelFocus || m.projectView) && !m.accountView && !m.creating && !m.panelVisible()) {
		right = strings.Split(m.previewRows(width, m.height), "\n")
	}
	main := strings.Split(content, "\n")
	lines := make([]string, max(0, m.height))
	for i := range lines {
		line := ""
		if len(left) > 0 {
			if i < len(left) {
				line = left[i]
			}
			line += strings.Repeat(" ", max(0, m.contentOffset()-ansi.StringWidth(line)))
		}
		if i < len(main) {
			line += main[i]
		}
		if i < len(right) {
			line += strings.Repeat(" ", max(0, m.contentOffset()+m.width+2-ansi.StringWidth(line))) + right[i]
		}
		line = clip(line, m.terminalWidth)
		lines[i] = line + strings.Repeat(" ", max(0, m.terminalWidth-ansi.StringWidth(line)))
	}
	return strings.Join(lines, "\n")
}
