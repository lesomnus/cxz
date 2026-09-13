package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Unlike a canonical ReadString, Bubble Tea understands bracketed paste,
// terminal responses and editing keys; none of those become confirmation text.
func ConfirmRecreate(ctx context.Context, in io.Reader, out io.Writer, target string) error {
	m := newRecreateConfirmation()
	m.target = target
	if file, ok := in.(*os.File); ok {
		in = keyboardInput(file)
	}
	_, err := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	if !m.confirmed {
		return fmt.Errorf("recreate canceled")
	}
	return nil
}

type recreateConfirmation struct {
	target    string
	input     textinput.Model
	message   string
	confirmed bool
}

func newRecreateConfirmation() *recreateConfirmation {
	in := textinput.New()
	in.Prompt = "Type recreate to continue: "
	in.Width = 20
	in.CharLimit = 100
	in.Focus()
	return &recreateConfirmation{input: in}
}
func (m *recreateConfirmation) Init() tea.Cmd { return textinput.Blink }
func (m *recreateConfirmation) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && !key.Paste {
		switch key.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
		case "enter":
			if strings.TrimSpace(m.input.Value()) == "recreate" {
				m.confirmed = true
				return m, tea.Quit
			}
			m.message = "Text does not match. Edit it or Esc to cancel."
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
func (m *recreateConfirmation) View() string {
	return accent.Bold(true).Render("Recreate project container") + "\n\n" +
		"Target: " + safeText(m.target) + "\n\n" +
		"The writable layer is removed; attached editors disconnect.\nWorkspace and named volumes are kept.\n\n" +
		m.input.View() + "\n" + warning.Render(m.message) + "\n\n" + muted.Render("Enter confirm · Esc cancel") + "\n"
}
