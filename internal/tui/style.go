package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	accent      = lipgloss.NewStyle().Foreground(lipgloss.Color("#24d17c"))
	brand       = lipgloss.NewStyle().Foreground(lipgloss.Color("#aeff98")).Background(lipgloss.Color("#031e2c")).Bold(true)
	teal        = lipgloss.NewStyle().Foreground(lipgloss.Color("#07898f"))
	lavender    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#7255A0", Dark: "#C9B6EE"})
	blue        = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#42758B", Dark: "#ACD6EB"})
	peach       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#A35D52", Dark: "#F2B8A7"})
	claude      = lipgloss.NewStyle().Foreground(lipgloss.Color("#D97757")).Bold(true)
	codex       = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#000000")).Bold(true)
	muted       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#626773", Dark: "#969BA8"})
	timestamp   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#777777", Dark: "#555B65"})
	answer      = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	strong      = lipgloss.NewStyle().Bold(true)
	warning     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#945600", Dark: "#EBC078"})
	selectedRow = accent.Bold(true)
)

func newComposer() textarea.Model {
	input := textarea.New()
	input.Placeholder = "Ask a question or describe a task… (/help)"
	input.Prompt = "› "
	input.SetPromptFunc(2, func(line int) string {
		if line == 0 {
			return "› "
		}
		return "  "
	})
	input.ShowLineNumbers = false
	input.CharLimit = 100000
	input.MaxHeight = 0
	input.MaxWidth = 0
	input.SetWidth(92)
	input.SetHeight(3)
	input.FocusedStyle.Prompt = accent
	input.BlurredStyle.Prompt = muted
	input.Focus()
	return input
}

// Drafts stay local to this TUI process and never cross session boundaries.
func (m *model) saveDraft() {
	if m.projectView || m.creating {
		return
	}
	if s := m.current(); s != nil {
		if m.drafts == nil {
			m.drafts = map[string]string{}
		}
		m.drafts[s.Id] = m.input.Value()
	}
}

func (m *model) restoreDraft() {
	m.input.Reset()
	if s := m.current(); s != nil {
		m.input.SetValue(m.drafts[s.Id])
	}
	m.resize()
}

func (m *model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	m.input.SetWidth(max(2, m.width-2))
	rows := 0
	for _, line := range strings.Split(m.input.Value(), "\n") {
		rows += max(1, (ansi.StringWidth(line)+max(1, m.width-4)-1)/max(1, m.width-4))
	}
	m.input.SetHeight(min(max(2, rows), min(6, max(1, m.height/4))))
	m.view.Width = max(1, m.width)
	// Status (1), composer border (2), bottom session information (1).
	m.view.Height = max(1, m.height-m.input.Height()-4)
}

func clip(s string, width int) string {
	return ansi.Truncate(s, max(1, width), "…")
}

func frame(body string, width int, highlighted bool) string {
	border := teal
	if highlighted {
		border = accent
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(border.GetForeground()).Width(max(1, width-2)).Render(body)
}

// Clip by display cells, not bytes: CJK text and ANSI styling stay intact.
func screen(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = clip(lines[i], width)
	}
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func (m *model) sessionScreen() string {
	width := max(1, m.width)
	info := brand.Render("cxz · sessions")
	if s := m.current(); s != nil {
		agent := pickerLabel(s.Agent)
		if s.Model != "" {
			agent += "/" + pickerLabel(s.Model)
		}
		title := pickerLabel(s.Title)
		if title == "" {
			title = fmt.Sprintf("%.8s", s.Id)
		}
		info += "  " + blue.Render(agent) + " · " + lavender.Render(pickerLabel(s.Account)) + " · " + teal.Render("["+pickerLabel(s.State)+"]") + " · " + title
	}
	if m.focusList {
		info = accent.Render("↔ ") + info
	}
	status := warning.Render(pickerLabel(m.notice))
	if s := m.current(); s != nil && len(s.Pending) > 0 {
		status = warning.Bold(true).Render("APPROVAL · F2 allow / F3 deny") + "  " + pickerLabel(s.Pending[0].Text)
	} else if !m.view.AtBottom() {
		status = muted.Render("Reading history") + "  " + status
	}
	body := m.view.View() + "\n" + clip(status, width) + "\n" +
		frame(m.input.View(), width, !m.focusList) + "\n" + clip(info, width)
	return screen(body, m.width, m.height)
}

func helpView(width int) string {
	return lavender.Bold(true).Render("cxz /help") + "\n" +
		muted.Render(ansi.Hardwrap(
			"Enter          Send message\n"+
				"Alt+Enter / Ctrl+J  Newline\n"+
				"Ctrl+X         Clear draft\n"+
				"Tab            Toggle session selection; ↑/↓ select, Enter open\n"+
				"Ctrl+Q         Return to project\n"+
				"Ctrl+N         Create session\n"+
				"F2 / F3        Allow / deny pending approval\n"+
				"F4             Interrupt active turn\n"+
				"Ctrl+R         Resume stopped session\n"+
				"PgUp / PgDn    Scroll conversation\n"+
				"Ctrl+C         Detach (agent continues)\n"+
				"/answer {\"question\":\"answer\"}  Reply to question\n"+
				"/stop          Stop agent\n"+
				"/help          Show this local help (not sent to agent)",
			max(1, width), true))
}
