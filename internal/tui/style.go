package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	accent      = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#006D77", Dark: "#76D7CB"})
	muted       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#626773", Dark: "#969BA8"})
	strong      = lipgloss.NewStyle().Bold(true)
	warning     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#945600", Dark: "#EBC078"})
	selectedRow = accent.Bold(true)
)

func newComposer() textarea.Model {
	input := textarea.New()
	input.Placeholder = "Ask a question or describe a task…"
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
	m.input.SetWidth(max(2, m.width-6))
	rows := 0
	for _, line := range strings.Split(m.input.Value(), "\n") {
		rows += max(1, (ansi.StringWidth(line)+max(1, m.width-8)-1)/max(1, m.width-8))
	}
	m.input.SetHeight(min(max(2, rows), min(6, max(1, m.height/4))))
	m.view.Width = max(1, m.width-4)
	// Header (3), approval/status (2), composer border (2), help (3).
	m.view.Height = max(1, m.height-m.input.Height()-10)
}

func clip(s string, width int) string {
	return ansi.Truncate(s, max(1, width), "…")
}

func frame(body string, width int, highlighted bool) string {
	border := muted
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
	width := max(1, m.width-4)
	title, detail := "Conversation", "Choose a session with Tab · ↑/↓ · Enter"
	if s := m.current(); s != nil {
		title = pickerLabel(s.Title)
		if title == "" {
			title = "Session " + fmt.Sprintf("%.8s", s.Id)
		}
		agent := pickerLabel(s.Agent)
		if s.Model != "" {
			agent += " / " + pickerLabel(s.Model)
		}
		detail = fmt.Sprintf("%s  ·  account %s  ·  %s", agent, pickerLabel(s.Account), pickerLabel(s.State))
	}
	heading := accent.Bold(true).Render("cxz · sessions") + "  /  " + strong.Render(title)
	if m.focusList {
		heading += accent.Render("  [↑/↓ select · Enter open]")
	}
	status := muted.Render("Agent continues when you detach.")
	if s := m.current(); s != nil && len(s.Pending) > 0 {
		status = warning.Bold(true).Render("APPROVAL · F2 allow / F3 deny") + "  " + pickerLabel(s.Pending[0].Text)
	} else if !m.view.AtBottom() {
		status = warning.Render("Reading history · PgDown to return to latest")
	}
	help := "Enter send · Alt+Enter/Ctrl+J newline · PgUp/PgDn history"
	controls := "Ctrl+Q project · Tab sessions · F4 interrupt · Ctrl+R resume · Ctrl+C detach"
	if s := m.current(); s != nil && len(s.Pending) > 0 {
		help = "F2 allow · F3 deny · /answer {\"question\":\"answer\"} · Enter send"
	}
	body := clip(heading, width) + "\n" + clip(muted.Render(detail), width) + "\n\n" +
		m.view.View() + "\n" + clip(status, width) + "\n" +
		frame(m.input.View(), width, !m.focusList) + "\n" +
		clip(muted.Render(help), width) + "\n" + clip(muted.Render(controls), width) + "\n" +
		clip(warning.Render(pickerLabel(m.notice)), width)
	return screen(lipgloss.NewStyle().Padding(0, 2).Render(body), m.width, m.height)
}
