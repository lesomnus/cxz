package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type slashCommand struct{ name, description string }

var slashCommands = []slashCommand{
	{"/help", "Keyboard shortcuts"},
	{"/context", "Context details · Unimplemented"},
	{"/compact", "Compact conversation · Unimplemented"},
	{"/usage", "Session tokens, cost and time"},
	{"/answer", "Reply to a pending question"},
	{"/stop", "Stop agent"},
}

func (m *model) commandHints() []slashCommand {
	text := m.input.Value()
	if m.creating || m.focusList || m.hintDismissed || !strings.HasPrefix(text, "/") || strings.ContainsAny(text, " \n\t") {
		return nil
	}
	var matches []slashCommand
	for _, c := range slashCommands {
		if strings.HasPrefix(c.name, text) {
			matches = append(matches, c)
		}
	}
	return matches
}

func (m *model) commandKey(k tea.KeyMsg) (bool, tea.Cmd) {
	hints := m.commandHints()
	if len(hints) > 0 {
		m.hintSelected = max(0, min(m.hintSelected, len(hints)-1))
		switch k.String() {
		case "up":
			m.hintSelected = (m.hintSelected + len(hints) - 1) % len(hints)
			return true, nil
		case "down":
			m.hintSelected = (m.hintSelected + 1) % len(hints)
			return true, nil
		case "esc":
			m.hintDismissed = true
			return true, nil
		case "tab":
			m.input.SetValue(hints[m.hintSelected].name)
			m.hintSelected = 0
			return true, nil
		case "enter":
			m.input.SetValue(hints[m.hintSelected].name)
			m.hintSelected = 0
			// Submission stays in the ordinary Enter handler.
			return false, nil
		}
	}
	if k.Type == tea.KeyRunes || k.String() == "backspace" || k.String() == "ctrl+x" {
		m.hintDismissed = false
		m.hintSelected = 0
	}
	return false, nil
}

// Replace the bottom visible transcript rows, without touching scroll position
// or saved events. The composer and status line never move for this overlay.
func (m *model) commandOverlay(view string) string {
	hints := m.commandHints()
	if len(hints) == 0 {
		return view
	}
	rows := strings.Split(view, "\n")
	count := min(len(hints), len(rows))
	selected := max(0, min(m.hintSelected, len(hints)-1))
	start := max(0, selected-count+1)
	for i := 0; i < count; i++ {
		c := hints[start+i]
		text := "  " + c.name + "  " + c.description
		style := muted
		if start+i == selected {
			text = "› " + c.name + "  " + c.description
			style = accent
		}
		line := clip(text, m.width)
		rows[len(rows)-count+i] = style.Render(line + strings.Repeat(" ", max(0, m.width-ansi.StringWidth(line))))
	}
	return strings.Join(rows, "\n")
}

func (m *model) localCommandView(id string) string {
	command := m.localOutput[id]
	if command == "/usage" {
		return indentBlock(ansi.Hardwrap(safeText(m.usageReports[id]), max(1, m.view.Width-2), true))
	}
	if command == "" || command == "/help" {
		return helpView(m.view.Width)
	}
	return indentBlock(peach.Render(command + " · Unimplemented"))
}
