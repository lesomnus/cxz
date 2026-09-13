package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func providerLabel(provider string) string {
	text := fmt.Sprintf("%-6s", clip(pickerLabel(provider), 6))
	switch provider {
	case "claude":
		return claude.Render(text)
	case "codex":
		return codex.Render(text)
	default:
		return lavender.Render(text)
	}
}

var (
	accent       = lipgloss.NewStyle().Foreground(lipgloss.Color("#24d17c"))
	brand        = lipgloss.NewStyle().Foreground(lipgloss.Color("#aeff98")).Background(lipgloss.Color("#000000")).Bold(true)
	teal         = lipgloss.NewStyle().Foreground(lipgloss.Color("#07898f"))
	lavender     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#7255A0", Dark: "#C9B6EE"})
	blue         = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#42758B", Dark: "#ACD6EB"})
	pinnedPrompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#ACD6EB")).Background(lipgloss.Color("#031e2c"))
	peach        = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#A35D52", Dark: "#F2B8A7"})
	claude       = lipgloss.NewStyle().Foreground(lipgloss.Color("#D97757")).Bold(true)
	codex        = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#000000")).Bold(true)
	muted        = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#626773", Dark: "#969BA8"})
	timestamp    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#777777", Dark: "#555B65"})
	metricStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#626975"))
	zeroStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#30343B"))
	answer       = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	strong       = lipgloss.NewStyle().Bold(true)
	warning      = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#945600", Dark: "#EBC078"})
	selectedRow  = accent.Bold(true)
)

func newComposer() textarea.Model {
	input := textarea.New()
	input.Placeholder = "Ask a question… (Ctrl+S to send · /help)"
	input.Prompt = "› "
	input.SetPromptFunc(2, func(line int) string {
		if line == 0 {
			return "> "
		}
		return timestamp.Render(fmt.Sprintf("%d ", line%10))
	})
	input.ShowLineNumbers = false
	input.CharLimit = 100000
	input.MaxHeight = 0
	input.MaxWidth = 0
	input.SetWidth(92)
	input.SetHeight(3)
	input.FocusedStyle.Prompt = accent
	input.BlurredStyle.Prompt = muted
	input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	input.BlurredStyle.CursorLine = lipgloss.NewStyle()
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
	m.interruptKey = ""
	m.approvalOffset = 0
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
	follow := m.view.AtBottom()
	m.input.SetWidth(max(2, m.width-2))
	rows := 0
	for _, line := range strings.Split(m.input.Value(), "\n") {
		rows += max(1, (ansi.StringWidth(line)+max(1, m.width-4)-1)/max(1, m.width-4))
	}
	m.input.SetHeight(min(max(2, rows), min(6, max(1, m.height/4))))
	// SetValue/SetHeight alone do not reveal a cursor below the old viewport.
	// Populate its new content, then let the widget re-anchor its scroll offset.
	_ = m.input.View()
	m.input, _ = m.input.Update(nil)
	m.view.Width = max(1, m.width)
	// Blank separator + status (2), composer border (2), session information (1).
	m.view.Height = max(1, m.height-m.input.Height()-5-m.approvalHeight())
	if follow {
		m.view.GotoBottom()
	}
}

func clip(s string, width int) string {
	return ansi.Truncate(s, max(1, width), "…")
}

func frame(body string, width int, highlighted bool) string {
	border := teal
	if highlighted {
		border = accent
	}
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(max(1, width-2))
	// Lip Gloss 1.x emits empty SGR parameters when both border colors are
	// configured on an uncolored renderer.
	if lipgloss.ColorProfile().Name() != "Ascii" {
		style = style.BorderForeground(border.GetForeground())
	}
	return style.Render(body)
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
	info := " -------"
	if s := m.current(); s != nil {
		agent := pickerLabel(s.Agent)
		if model := m.selectedModelLabel(); model != "" {
			agent += "/" + model
		}
		title := pickerLabel(s.Title)
		if title == "" {
			title = "Untitled"
		}
		alias := fmt.Sprintf("%-7s", clip(pickerLabel(safeText(s.Alias)), 7))
		if s.Alias == "" {
			alias = "-------"
		}
		if m.renaming {
			alias = m.aliasInput.View()
		}
		indicator := " "
		if m.focusList {
			indicator = accent.Render("›")
		}
		info = indicator + accent.Render(alias) + "  " + blue.Render(agent) + " · " + lavender.Render(pickerLabel(s.Account)) + " · " + title
	}
	status := warning.Render(pickerLabel(m.notice))
	if !m.view.AtBottom() {
		status = muted.Render(m.scrollStatus())
		if m.fullPermissionNotice() != "" {
			status = warning.Render("FULL · ") + status
		}
	} else if full := m.fullPermissionNotice(); full != "" {
		status = warning.Render(full)
		if m.notice != "" {
			status += " · " + warning.Render(pickerLabel(m.notice))
		}
	} else if s := m.current(); s != nil && s.State != "working" && s.State != "idle" && s.State != "waiting_input" {
		status = warning.Render(pickerLabel(s.State)) + " · " + status
	}
	box := m.approvalBox()
	if s := m.current(); s != nil && s.State == "waiting_input" && m.interruptKey == s.Id+"/"+s.RunId && time.Now().Before(m.interruptUntil) {
		status = warning.Render("Esc again to interrupt (3s)")
	}
	if box != "" {
		box += "\n"
	}
	quota := m.quotaStatus(time.Now(), max(1, width-13))
	leftWidth := max(8, width-ansi.StringWidth(quota)-3)
	info = clip(info, leftWidth)
	info += strings.Repeat(" ", max(1, width-1-ansi.StringWidth(info)-ansi.StringWidth(quota))) + quota + " "
	body := m.commandOverlay(m.conversationView()) + "\n\n" + clip("  "+status, width) + "\n" + box +
		frame(m.input.View(), width, !m.focusList && !m.focusApproval) + "\n" + clip(info, width)
	return screen(body, m.width, m.height)
}

func indentBlock(s string) string {
	if s == "" {
		return ""
	}
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}
