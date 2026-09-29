package tui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/lesomnus/cxz/internal/core"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

const maxViewWidth = 133
const projectPanelWidth = 36
const minProjectPanelWidth = 28
const projectPanelGap = 2
const projectPanelHeaderRows = 4
const projectPanelFooterRows = 7

var panelSeparatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

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
	if c, ok := m.client.(interface{ DefaultConnection() string }); ok && project == nil && id == "" {
		m.panelWantConnection = c.DefaultConnection()
	}
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
	return m.terminalWidth >= 40+minProjectPanelWidth+projectPanelGap && m.height >= 14
}
func (m *model) panelWidth() int {
	return min(projectPanelWidth, max(minProjectPanelWidth, m.terminalWidth/4))
}
func (m *model) panelScreenWidth() int {
	if m.panelVisible() {
		return m.panelWidth()
	}
	if m.terminalWidth > 0 {
		return m.terminalWidth
	}
	return m.width
}
func (m *model) contentOffset() int {
	if m.panelVisible() {
		return m.panelWidth() + projectPanelGap
	}
	return 0
}

// Keep navigation indices tied to real items. Decorative separators occupy
// screen rows only, shared by rendering, scrolling and mouse hit testing.
func panelLayout(rows []panelRow) []int {
	var layout []int
	for i, row := range rows {
		if i > 0 && row.session == nil {
			layout = append(layout, -1)
		}
		layout = append(layout, i)
	}
	return layout
}

func (m *model) panelPosition(layout []int) int {
	for i, item := range layout {
		if item == m.panelIndex {
			return i
		}
	}
	return 0
}

func (m *model) panelRange(layout []int) (start, end int) {
	capacity := max(1, m.height-projectPanelHeaderRows-projectPanelFooterRows)
	start = max(0, min(m.panelPosition(layout)-capacity+3, len(layout)-capacity))
	return start, min(len(layout), start+capacity)
}

func (m *model) panelMouse(v tea.MouseMsg) (bool, tea.Cmd) {
	m.panelHoverY = 0
	m.panelHintHover = ""
	if m.height < 14 || m.width < 40 || m.settingsPage != nil || (m.memoryPage != nil || m.library != nil) ||
		m.workflow != nil || m.accountView || m.creating || m.redactDialog != nil ||
		m.questionFocused() || m.pasteDialog != nil || m.restartConfirm != nil || m.modelPicker != nil || m.report != nil {
		return false, nil
	}
	if !m.panelVisible() && !m.panelFocus && !m.projectView {
		return false, nil
	}
	if v.X < 0 || v.X >= m.panelScreenWidth() || v.Y < 0 || v.Y >= m.height {
		return false, nil
	}
	if m.busy || m.renaming {
		return true, nil
	}
	// A footer hint is pressed rather than handled here: sending its key keeps the
	// click on the same path as the keystroke, including the routing that decides
	// whether the panel or the conversation owns it.
	footer := m.panelHints()
	if row := v.Y - m.panelHintY(0); row >= 0 && row < len(footer) {
		hint := hintAt(footer[row], v.X-panelHintX)
		if !hint.actionable() {
			return true, nil
		}
		m.panelHintHover = hint.key
		if v.Action == tea.MouseActionPress && v.Button == tea.MouseButtonLeft {
			m.focusPanel()
			press := hint.press
			return true, func() tea.Msg { return press }
		}
		return true, nil
	}
	rows := m.panelRows()
	layout := panelLayout(rows)
	start, end := m.panelRange(layout)
	position := start + v.Y - projectPanelHeaderRows
	index := -1
	if v.Y >= projectPanelHeaderRows && position < end {
		index = layout[position]
	}
	onItem := index >= 0
	if onItem {
		m.panelHoverY = v.Y
	}
	switch {
	case v.Action == tea.MouseActionPress && v.Button == tea.MouseButtonLeft:
		if v.Y >= projectPanelHeaderRows && position < end && !onItem {
			return true, nil // The divider never selects or activates an item.
		}
		m.focusPanel()
		if onItem {
			m.panelIndex = index
			return true, m.panelKey(tea.KeyMsg{Type: tea.KeyEnter})
		}
	case v.Button == tea.MouseButtonWheelUp || v.Button == tea.MouseButtonWheelDown:
		if !m.panelFocus {
			m.focusPanel()
		}
		m.panelWantConnection = ""
		delta := 3
		if v.Button == tea.MouseButtonWheelUp {
			delta = -delta
		}
		m.panelIndex = max(0, min(max(0, len(rows)-1), m.panelIndex+delta))
	}
	return true, nil
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
		// Down keeps the registry and sessions for recovery, but removes the
		// project from the navigation list until it is brought up again.
		if p.State == "absent" && p.ProvisionState == "complete" && p.ProvisionStep == "down" {
			continue
		}
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
		message := "Project list unavailable: " + v.projectsErr.Error()
		if m.panelError != message {
			m.showError(message)
		}
		m.panelError = message
	} else if v.projectsLoaded || v.projects != nil {
		m.panelProjects = v.projects
		m.panelError = ""
	}
	rows = m.panelRows()
	if m.panelWantKey != "" {
		key = m.panelWantKey
	}
	if m.panelWantConnection != "" {
		for _, r := range rows {
			if m.connectionName(r.project.Id) == m.panelWantConnection {
				key = r.key()
				if r.project.State != "connection" {
					m.panelWantConnection = ""
				}
				break
			}
		}
	}
	m.panelIndex = max(0, min(m.panelIndex, len(rows)-1))
	for i, r := range rows {
		if r.key() == key {
			m.panelIndex = i
			m.panelWantKey = ""
			break
		}
	}
	m.validateDeleteSelection()
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

func (m *model) focusConversationMouse(v tea.MouseMsg) {
	wheel := v.Button == tea.MouseButtonWheelUp || v.Button == tea.MouseButtonWheelDown
	if (!m.panelFocus && !wheel) || m.projectView || m.accountView || m.creating ||
		m.workflow != nil || m.settingsPage != nil || (m.memoryPage != nil || m.library != nil) || m.busy || m.renaming ||
		v.Action != tea.MouseActionPress || (!wheel && v.Button != tea.MouseButtonLeft) ||
		v.X < m.contentOffset() || v.X >= m.contentOffset()+m.width || v.Y < 0 || v.Y >= m.height {
		return
	}
	if wheel && v.Y >= m.view.Height {
		return
	}
	if d := m.questionDialog; d != nil {
		d.suspended = true
	}
	m.panelFocus, m.focusList, m.focusApproval = false, false, false
	m.toolSelector = nil
	if m.filePreview != nil {
		m.filePreview.focused = false
	}
	if p := m.terminal(); p != nil {
		p.focused = false
	}
	m.input.Focus()
}

func (m *model) panelKey(k tea.KeyMsg) tea.Cmd {
	if k.Paste {
		return nil
	}
	if k.String() != "ctrl+x" {
		m.deleteConfirm = nil
	}
	if k.String() == "ctrl+d" {
		return tea.Quit
	}
	if m.busy {
		return nil
	}
	rows := m.panelRows()
	m.panelWantConnection = ""
	switch k.String() {
	case "ctrl+d":
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
	case "tab", "shift+tab":
		if m.errorVisible() {
			m.focusError()
		}
	case "up":
		m.panelIndex = max(0, m.panelIndex-1)
	case "down":
		m.panelIndex = min(max(0, len(rows)-1), m.panelIndex+1)
	case "home":
		m.panelIndex = 0
	case "end":
		m.panelIndex = max(0, len(rows)-1)
	case "pgup", "pgdown":
		layout := panelLayout(rows)
		if len(layout) == 0 {
			break
		}
		direction := 1
		if k.String() == "pgup" {
			direction = -1
		}
		position := max(0, min(len(layout)-1, m.panelPosition(layout)+direction*max(1, m.height-projectPanelHeaderRows-projectPanelFooterRows)))
		if layout[position] < 0 {
			position += direction
		}
		m.panelIndex = layout[position]
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
	case "r":
		if len(rows) == 0 || rows[m.panelIndex].session == nil {
			m.notice = "Select a session to rename."
			return nil
		}
		return m.renameSession(rows[m.panelIndex].session)
	case "m":
		if len(rows) == 0 || rows[m.panelIndex].session == nil {
			m.notice = "Select a session to inspect retained agent data."
			return nil
		}
		return m.openMemory(rows[m.panelIndex].session)
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
	case "ctrl+x":
		return m.confirmSessionDelete(time.Now())
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
				if c, ok := m.client.(interface{ ConnectionError(string) string }); ok {
					if err := c.ConnectionError(r.project.Id); err != "" {
						m.showError(err)
					}
				}
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
	m.saveQuestionDraft()
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

var panelStyleSequence = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

func panelBackground(line string) string {
	return panelRowBackground(line, 236)
}

// Use ANSI grayscale indices so RGB and 256-color terminals show the same fill.
func panelRowBackground(line string, shade int) string {
	profile := lipgloss.ColorProfile()
	if profile.Name() == "Ascii" {
		return line
	}
	gray := 8 + (shade-232)*10
	background := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", gray, gray, gray)
	if profile.Name() == "ANSI256" {
		background = fmt.Sprintf("\x1b[48;5;%dm", shade)
	}
	if profile.Name() == "ANSI" {
		// Preserve a visible gray fill when only the 16-color palette is available.
		background = "\x1b[100m"
		if shade == 238 {
			background = "\x1b[47m"
		}
	}
	// Child labels end with SGR resets, and some badges set their own background.
	// Restore the panel background after every generated style change, including
	// resets before the padded spaces, then reset at the actual panel boundary.
	line = panelStyleSequence.ReplaceAllStringFunc(line, func(style string) string { return style + background })
	return background + line + "\x1b[0m"
}

func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// The footer offers only what the selected row can do: renaming, stopping,
// inspecting and deleting each need a session, and creating one needs a project.
// panelKey refuses the same cases with a notice, so this only stops the row from
// advertising a key that would do nothing. "agents run" is what detaching leaves
// behind rather than a key, so it is a note.
//
// Rows are ordered as drawn; panelHintY turns an index into a screen row.
func (m *model) panelHints() [][]hintSpec {
	rows := m.panelRows()
	noProject := len(rows) == 0
	noSession := noProject || m.panelIndex < 0 || m.panelIndex >= len(rows) || rows[m.panelIndex].session == nil
	return [][]hintSpec{
		{{key: "n", label: "new", press: runeKey('n'), disabled: noProject},
			{key: "a", label: "accounts", press: runeKey('a'), disabled: noProject}},
		{{key: "r", label: "rename", press: runeKey('r'), disabled: noSession},
			{key: "s", label: "stop", press: runeKey('s'), disabled: noSession}},
		{{key: "Ctrl+X twice", label: "delete", press: tea.KeyMsg{Type: tea.KeyCtrlX}, disabled: noSession}},
		{{key: "m", label: "memory", press: runeKey('m'), disabled: noSession},
			// Ctrl+. reaches the TUI as F19; see the terminal key bridge.
			{key: "Ctrl+.", label: "settings", press: tea.KeyMsg{Type: tea.KeyF19}}},
		{{key: "Esc/Ctrl+Q", label: "return", press: tea.KeyMsg{Type: tea.KeyEsc}}},
		{{key: "Ctrl+D", label: "detach", press: tea.KeyMsg{Type: tea.KeyCtrlD}},
			{label: "agents run", note: true}},
	}
}

// The footer is drawn flush to the bottom above the status row, and every panel
// line carries one leading space, so a hint starts at column 1.
func (m *model) panelHintY(row int) int { return m.height - projectPanelFooterRows + row }

const panelHintX = 1

func (m *model) panelScreen() string {
	width := m.panelScreenWidth()
	rows := m.panelRows()
	layout := panelLayout(rows)
	start, end := m.panelRange(layout)
	lines := []string{"", accent.Bold(true).Render("Projects"), muted.Render("↑/↓ select · Enter open"), ""}
	if m.deletingID != "" {
		lines[2] = warning.Render(workingSpinner(m.pulse) + " Deleting session…")
	} else if d := m.deleteConfirm; d != nil && time.Now().Before(d.until) {
		seconds := int(time.Until(d.until).Seconds()) + 1
		lines[2] = warning.Render(fmt.Sprintf("Ctrl+X again · delete (%ds)", seconds))
	}
	if len(rows) == 0 {
		lines = append(lines, "No projects")
	}
	for position := start; position < end; position++ {
		i := layout[position]
		if i < 0 {
			lines = append(lines, panelSeparatorStyle.Render(strings.Repeat("─", max(0, width-2))))
			continue
		}
		r := rows[i]
		label := r.project.Name
		via := m.connectionLabel(r.project.Id)
		if via != "" {
			label = strings.TrimSuffix(label, via)
		}
		if label == "" {
			label = r.project.Alias
		}
		if label == "" {
			label = r.project.Id
		}
		if r.project.Alias != "" && r.project.Alias != label {
			label += " · " + r.project.Alias
		}
		line := lavender.Render(pickerLabel(label))
		if via != "" {
			line += metricStyle.Render(" via " + pickerLabel(strings.TrimPrefix(via, " via ")))
		}
		if s := r.session; s != nil {
			name := s.Alias
			if name == "" {
				name = s.Id
			}
			sessionName := pickerLabel(name)
			if m.renaming && m.renameID == s.Id {
				sessionName = m.aliasInput.View()
			}
			line = m.sessionIndicator(s) + " " + clip(sessionName, max(1, width-17)) + " · " + providerLabel(s.Agent)
			if s.State != "idle" && !workingState(s.State) && s.State != "" {
				line += " · " + pickerLabel(s.State)
			}
		}
		prefix := "  "
		if current := m.current(); current != nil && !m.projectView && r.session != nil && current.Id == r.session.Id {
			prefix = "• "
		}
		if i == m.panelIndex && m.panelFocus {
			prefix = "› "
		}
		lines = append(lines, prefix+line)
	}
	for len(lines) < m.height-projectPanelFooterRows {
		lines = append(lines, "")
	}
	status := ""
	// The standalone project page has no composer. The sidebar leaves notices
	// to the conversation's status row so errors appear only once.
	if !m.panelVisible() {
		status = m.navigationNotice()
	}
	if m.busy {
		status = "Working…"
	}
	for _, row := range m.panelHints() {
		lines = append(lines, hintRow(m.panelHintHover, row))
	}
	lines = append(lines, warning.Render(pickerLabel(status)))
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	for i := range lines {
		line := clip(lines[i], max(1, width-2))
		shade := 236
		if i >= projectPanelHeaderRows && i < projectPanelHeaderRows+end-start {
			item := layout[start+i-projectPanelHeaderRows]
			if item >= 0 && i == m.panelHoverY {
				shade = 237
			}
			if item >= 0 && m.panelFocus && item == m.panelIndex {
				shade = 238
			}
		}
		lines[i] = panelRowBackground(" "+line+strings.Repeat(" ", max(0, width-2-ansi.StringWidth(line)))+" ", shade)
	}
	if m.panelVisible() && m.panelFocus {
		lines[m.height-1] = panelBackground(accent.Render(strings.Repeat("─", max(0, width))))
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
