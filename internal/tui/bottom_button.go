package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func (m *model) bottomButton() (x, y int, label string) {
	if m.current() == nil || m.projectView || m.accountView || m.settingsPage != nil || (m.memoryPage != nil || m.library != nil) || !m.previewInteraction() || m.errorFocused() || m.historyShimmer != nil || m.historyOpening[m.watchID] || m.view.Height < 1 {
		return 0, 0, ""
	}
	if m.view.AtBottom() && !m.historyWindow(m.current().Id).detached {
		return 0, 0, ""
	}
	label = " ↓ Latest "
	if m.width < ansi.StringWidth(label)+2 {
		label = " ↓ "
	}
	if m.width < ansi.StringWidth(label) {
		return 0, 0, ""
	}
	return max(0, m.width-ansi.StringWidth(label)-1), m.view.Height - 1, label
}

func (m *model) bottomButtonView(rows []string) {
	x, y, label := m.bottomButton()
	if label != "" && y < len(rows) {
		rows[y] = overlayButton(rows[y], x, label, m.bottomButtonHover, 238)
	}
}

// Coordinates are relative to the conversation, after the project panel offset.
func (m *model) bottomButtonMouse(v tea.MouseMsg) (bool, tea.Cmd) {
	x, y, label := m.bottomButton()
	if label == "" || v.Y != y || v.X < x || v.X >= x+ansi.StringWidth(label) || v.Button == tea.MouseButtonWheelUp || v.Button == tea.MouseButtonWheelDown {
		return false, nil
	}
	m.bottomButtonHover = true
	m.toolHover = nil
	if v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress {
		m.textSelection = nil
		m.toolSelector = nil
		m.toolClick = nil
		m.bottomButtonHover = false
		return true, m.goToLatest()
	}
	return true, nil
}

func (m *model) goToLatest() tea.Cmd {
	m.view.GotoBottom()
	return m.requestNewerHistory(true)
}
