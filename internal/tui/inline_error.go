package tui

import (
	"context"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// InlineError uses the normal screen, preserving the surrounding terminal output.
// It returns true only after deliberate activation of the Trust button.
func InlineError(ctx context.Context, in io.Reader, out io.Writer, message string, trust bool) (bool, error) {
	m := &inlineError{message: message, trust: trust, width: 80, height: 24}
	if file, ok := in.(*os.File); ok {
		in = keyboardInput(file)
	}
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out))
	_, err := runKeyboardProgram(ctx, p, in, out, nil)
	return m.accepted, err
}

type inlineError struct {
	message                         string
	trust, accepted, done           bool
	selected, width, height, offset int
}

func (m *inlineError) Init() tea.Cmd { return nil }
func (m *inlineError) lines() ([]string, int) {
	width := max(1, min(96, m.width)-4)
	lines := strings.Split(ansi.Hardwrap(safeText(m.message), width, true), "\n")
	return lines, max(1, min(12, m.height-7))
}
func (m *inlineError) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
	case tea.KeyMsg:
		if v.Paste {
			return m, nil
		}
		switch v.String() {
		case "ctrl+c", "esc":
			m.done = true
			return m, tea.Quit
		case "up", "down", "left", "right", "tab", "shift+tab":
			if m.trust {
				m.selected = 1 - m.selected
			}
		case "pgup":
			m.offset = max(0, m.offset-5)
		case "pgdown":
			m.offset += 5
		case "home":
			m.offset = 0
		case "end":
			m.offset = 1 << 20
		case "enter":
			m.accepted = m.trust && m.selected == 1
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}
func (m *inlineError) View() string {
	if m.done {
		if m.accepted {
			return "Configuration trusted; continuing…\n"
		}
		return safeText(m.message) + "\nExited.\n"
	}
	lines, capacity := m.lines()
	m.offset = max(0, min(m.offset, max(0, len(lines)-capacity)))
	title := "Error"
	if m.trust {
		title = "Trust this project configuration?"
	}
	out := []string{accent.Bold(true).Render(title), ""}
	out = append(out, lines[m.offset:min(len(lines), m.offset+capacity)]...)
	if len(lines) > capacity {
		out = append(out, muted.Render("PgUp/PgDn scroll for full details"))
	}
	out = append(out, "")
	labels := []string{"Exit"}
	if m.trust {
		labels = append(labels, "Trust")
	}
	var buttons []string
	for i, label := range labels {
		text := "  [ " + label + " ]"
		if m.selected == i {
			text = accent.Bold(true).Render("› [ " + label + " ]")
		}
		buttons = append(buttons, text)
	}
	out = append(out, strings.Join(buttons, "   "))
	out = append(out, muted.Render("←→ / Tab choose · Enter select · Esc exit"))
	return indentBlock(strings.Join(out, "\n")) + "\n"
}
