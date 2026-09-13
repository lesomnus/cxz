package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type restartButton struct {
	x, y, width int
	confirm     bool
}

// Share layout with hit testing. Keep both buttons visible in small viewports.
func (m *model) restartLayout(height int) ([]string, []restartButton) {
	if m.width < 15 || height < 4 {
		return nil, nil
	}
	p := m.restartConfirm
	label := p.id
	if s := m.current(); s != nil && s.Id == p.id && s.Alias != "" {
		label = s.Alias
	}
	body := []string{accent.Bold(true).Render("Restart agent · " + label)}
	text := "Active work stops; pending approvals are cleared.\nSession history and login stay. Automatic approval resets.\nThis does not update the runtime or recreate the container."
	body = append(body, strings.Split(ansi.Hardwrap(text, m.width-4, true), "\n")...)
	body = append(body, muted.Render("Tab / Shift+Tab / arrows choose · Enter select · Esc cancel"))
	button := func(label string, selected bool) string {
		if selected {
			return accent.Reverse(true).Bold(true).Render(label)
		}
		return muted.Render(label)
	}
	confirm, cancel := button("[ Confirm ]", p.confirm), button("[ Cancel ]", !p.confirm)
	buttonLines := []string{confirm + "  " + cancel}
	if m.width-4 < 23 {
		buttonLines = []string{confirm, cancel}
	}
	body = body[:min(len(body), height-2-len(buttonLines))]
	lines := append(body, buttonLines...)
	y := height - len(buttonLines) - 1
	buttons := []restartButton{{2, y, 11, true}, {15, y, 10, false}}
	if len(buttonLines) == 2 {
		buttons[1] = restartButton{2, y + 1, 10, false}
	}
	return lines, buttons
}

func (m *model) restartOverlay(view string) string {
	if m.restartConfirm == nil {
		return view
	}
	lines, _ := m.restartLayout(len(strings.Split(view, "\n")))
	if lines == nil {
		lines = []string{"Enlarge terminal; Esc cancels"}
	}
	return overlayBox(view, lines, m.width, true)
}

func (m *model) cancelRestart() tea.Cmd {
	m.restartConfirm = nil
	m.notice = "Restart canceled."
	return nil
}

func (m *model) restartKey(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc", "ctrl+q":
		return m.cancelRestart()
	case "ctrl+c":
		m.restartConfirm = nil
		return tea.Quit
	case "tab", "shift+tab", "left", "right", "up", "down":
		m.restartConfirm.confirm = !m.restartConfirm.confirm
	case "enter":
		if !m.restartConfirm.confirm {
			return m.cancelRestart()
		}
		if _, buttons := m.restartLayout(m.view.Height); len(buttons) > 0 {
			return m.confirmRestart()
		}
	}
	return nil
}

func (m *model) restartMouse(v tea.MouseMsg) tea.Cmd {
	if v.Action != tea.MouseActionPress || v.Button != tea.MouseButtonLeft {
		return nil
	}
	_, buttons := m.restartLayout(m.view.Height)
	for _, b := range buttons {
		if v.Y == b.y && v.X >= b.x && v.X < b.x+b.width {
			if b.confirm {
				return m.confirmRestart()
			}
			return m.cancelRestart()
		}
	}
	return nil
}
