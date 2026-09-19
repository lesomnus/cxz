package tui

import (
	"sort"
	"strings"

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
		for _, s := range ProjectSessions(m.allSessions, p) {
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
	m.panelIndex = max(0, min(m.panelIndex, len(rows)-1))
	for i, r := range rows {
		if r.key() == key {
			m.panelIndex = i
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
	rows := m.panelRows()
	switch k.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc", "ctrl+q":
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
		m.panelIndex = max(0, m.panelIndex-max(1, m.height-6))
	case "pgdown":
		m.panelIndex = min(max(0, len(rows)-1), m.panelIndex+max(1, m.height-6))
	case "left":
		for m.panelIndex > 0 && rows[m.panelIndex].session != nil {
			m.panelIndex--
		}
	case "right":
		if m.panelIndex+1 < len(rows) && rows[m.panelIndex].session == nil && rows[m.panelIndex+1].session != nil {
			m.panelIndex++
		}
	case "enter":
		if len(rows) == 0 {
			return nil
		}
		r := rows[m.panelIndex]
		m.backToProject()
		m.project = r.project
		m.sessions = ProjectSessions(m.allSessions, r.project)
		m.selected = 0
		m.questionDialog = nil
		m.pasteDialog = nil
		m.modelPicker = nil
		m.report = nil
		m.restartConfirm = nil
		m.accountView = false
		m.panelFocus = false
		m.projectView = true
		if r.session != nil {
			for i, s := range m.sessions {
				if s.Id == r.session.Id {
					m.selected = i
					break
				}
			}
			m.projectView = false
			m.restoreDraft()
			m.view.GotoBottom()
			m.watch()
		}
		m.resize()
		m.render()
		return tea.Batch(m.input.Focus(), m.refresh())
	}
	return nil
}

func (m *model) panelScreen() string {
	rows := m.panelRows()
	capacity := max(1, m.height-6)
	start := max(0, min(m.panelIndex-capacity+3, len(rows)-capacity))
	lines := []string{accent.Bold(true).Render("Projects"), muted.Render("↑/↓ select · Enter open"), ""}
	if len(rows) == 0 {
		lines = append(lines, muted.Render("No projects"))
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
		label = clip(pickerLabel(label), projectPanelWidth-5)
		line := lavender.Render(label)
		if s := r.session; s != nil {
			name := s.Alias
			if name == "" {
				name = s.Id
			}
			line = "  " + clip(pickerLabel(name), 7) + " · " + providerLabel(s.Agent)
			if s.State == "working" {
				line += " " + accent.Render("•")
			}
		}
		prefix := "  "
		if current := m.current(); current != nil && r.session != nil && current.Id == r.session.Id {
			prefix = accent.Render("• ")
		}
		if i == m.panelIndex && m.panelFocus {
			prefix = accent.Render("› ")
			line = accent.Bold(true).Render(ansi.Strip(line))
		}
		lines = append(lines, prefix+line)
	}
	for len(lines) < m.height-3 {
		lines = append(lines, "")
	}
	footer := "Ctrl+Q projects"
	if m.panelFocus {
		footer = "Esc return · Tab stays"
	}
	if m.panelError != "" {
		footer = m.panelError
	}
	lines = append(lines, muted.Render(clip(footer, projectPanelWidth-2)))
	return frame(strings.Join(lines, "\n"), projectPanelWidth, m.panelFocus)
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
	if width := m.previewSideWidth(); width > 0 {
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
