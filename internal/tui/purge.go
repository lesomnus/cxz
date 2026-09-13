package tui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/internal/purge"
)

type purgeModel struct {
	plan                 purge.Plan
	selected             map[string]bool
	focus, width, height int
	confirmed            bool
	notice               string
	details              viewport.Model
}

func newPurgeModel(p purge.Plan) *purgeModel {
	m := &purgeModel{plan: p, selected: map[string]bool{}, width: 90, height: 30, details: viewport.New(86, 12)}
	for _, g := range purge.Groups {
		m.selected[g.ID] = true
	}
	m.refreshDetails()
	return m
}
func (m *purgeModel) refreshDetails() {
	var lines []string
	for _, t := range m.plan.Targets {
		if m.selected[t.Group] {
			lines = append(lines, t.Kind+"  "+safeText(t.Name))
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "No selected targets.")
	}
	if len(m.plan.Preserved) > 0 {
		lines = append(lines, "", "Unknown local entries preserved:")
		for _, p := range m.plan.Preserved {
			lines = append(lines, safeText(p))
		}
	}
	m.details.SetContent(ansi.Hardwrap(strings.Join(lines, "\n"), max(1, m.details.Width), true))
}
func (m *purgeModel) Init() tea.Cmd { return nil }
func (m *purgeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = min(v.Width, maxViewWidth)
		m.height = v.Height
		m.details.Width = max(1, v.Width-4)
		m.details.Height = max(1, v.Height-16)
		m.refreshDetails()
	case tea.KeyMsg:
		if (m.width < 60 || m.height < 20) && v.String() != "esc" && v.String() != "ctrl+c" {
			return m, nil
		}
		switch v.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "up", "shift+tab":
			m.focus = (m.focus + len(purge.Groups)) % (len(purge.Groups) + 1)
		case "down", "tab":
			m.focus = (m.focus + 1) % (len(purge.Groups) + 1)
		case " ":
			if m.focus < len(purge.Groups) {
				id := purge.Groups[m.focus].ID
				m.selected[id] = !m.selected[id]
				m.notice = ""
				m.refreshDetails()
			}
		case "enter":
			if m.focus < len(purge.Groups) {
				id := purge.Groups[m.focus].ID
				m.selected[id] = !m.selected[id]
				m.refreshDetails()
				return m, nil
			}
			if err := m.plan.Validate(m.selected); err != nil {
				m.notice = err.Error()
				return m, nil
			}
			m.confirmed = true
			return m, tea.Quit
		case "pgup", "pgdown":
			m.details, _ = m.details.Update(msg)
		}
	}
	return m, nil
}
func (m *purgeModel) View() string {
	if m.width < 60 || m.height < 20 {
		return screen("cxz purge\nResize terminal to 60 × 20 to review deletion.\nEsc cancels; nothing deleted.", m.width, m.height)
	}
	var b strings.Builder
	b.WriteString(warning.Bold(true).Render("cxz purge · permanent deletion") + "\n")
	b.WriteString("State: " + safeText(m.plan.Root) + "\n")
	b.WriteString("Workspace source, personal agent logins, shared images/cache and CLI binary stay.\n\n")
	for i, g := range purge.Groups {
		check := "[ ]"
		if m.selected[g.ID] {
			check = "[x]"
		}
		marker := "  "
		if m.focus == i {
			marker = "› "
		}
		n := 0
		for _, t := range m.plan.Targets {
			if t.Group == g.ID {
				n++
			}
		}
		line := fmt.Sprintf("%s%s %s (%d)", marker, check, g.Label, n)
		if m.focus == i {
			line = accent.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + m.details.View() + "\n")
	label := "  [ Confirm irreversible deletion ]"
	if m.focus == len(purge.Groups) {
		label = warning.Bold(true).Render("› [ Confirm irreversible deletion ]")
	}
	b.WriteString(label + "\n")
	b.WriteString(muted.Render("Space toggle · ↑/↓ select · PgUp/PgDn targets · Esc cancel") + "\n")
	b.WriteString(warning.Render(safeText(m.notice)))
	return screen(b.String(), m.width, m.height)
}

func ConfirmPurge(ctx context.Context, p purge.Plan, in io.Reader, out io.Writer) (map[string]bool, bool, error) {
	m := newPurgeModel(p)
	_, err := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen()).Run()
	return m.selected, m.confirmed, err
}
