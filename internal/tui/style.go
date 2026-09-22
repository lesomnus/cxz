package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
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
	accent           = lipgloss.NewStyle().Foreground(lipgloss.Color("#24d17c"))
	magenta          = lipgloss.NewStyle().Foreground(lipgloss.Color("#ED79D4"))
	inputCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#AEFF98"))
	brand            = lipgloss.NewStyle().Foreground(lipgloss.Color("#aeff98")).Background(lipgloss.Color("#000000")).Bold(true)
	teal             = lipgloss.NewStyle().Foreground(lipgloss.Color("#07898f"))
	lavender         = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#7255A0", Dark: "#C9B6EE"})
	blue             = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#42758B", Dark: "#ACD6EB"})
	peach            = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#A35D52", Dark: "#F2B8A7"})
	failure          = lipgloss.NewStyle().Foreground(lipgloss.Color("#F26D78"))
	claude           = lipgloss.NewStyle().Foreground(lipgloss.Color("#D97757")).Bold(true)
	codex            = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#000000")).Bold(true)
	muted            = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#626773", Dark: "#969BA8"})
	timestamp        = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#777777", Dark: "#555B65"})
	metricStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#626975"))
	zeroStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#30343B"))
	answer           = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	strong           = lipgloss.NewStyle().Foreground(lipgloss.Color("253")).Bold(true)
	warning          = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#945600", Dark: "#EBC078"})
	selectedRow      = accent.Bold(true)
)

func newComposer() textarea.Model {
	input := textarea.New()
	input.KeyMap.WordBackward = key.NewBinding(key.WithKeys("ctrl+left", "alt+left", "alt+b"))
	input.KeyMap.WordForward = key.NewBinding(key.WithKeys("ctrl+right", "alt+right", "alt+f"))
	input.Cursor.Style = inputCursorStyle
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
	if m.terminalWidth > 0 {
		available := m.terminalWidth - m.contentOffset()
		m.width = min(maxViewWidth, available)
		if m.previewVisible() && available >= 140 {
			m.width = min(m.width, available-52)
		}
	}
	m.input.SetWidth(max(2, m.width-2))
	rows := 0
	for _, line := range strings.Split(m.input.Value(), "\n") {
		rows += max(1, (ansi.StringWidth(line)+max(1, m.width-4)-1)/max(1, m.width-4))
	}
	m.input.SetHeight(min(max(2, rows), min(6, max(1, m.height/4), max(1, m.height-6-m.errorHeight()))))
	// SetValue/SetHeight alone do not reveal a cursor below the old viewport.
	// Populate its new content, then let the widget re-anchor its scroll offset.
	_ = m.input.View()
	m.input, _ = m.input.Update(nil)
	viewWidth := max(1, m.width-2)
	if m.view.Width != viewWidth {
		m.renderedTools = nil
		m.renderedInputs = nil
		m.renderedSummaries = nil
	}
	m.view.Width = viewWidth
	if m.liveRenderWidth != nil {
		m.liveRenderWidth.Store(int64(viewWidth))
	}
	// Blank separator + status (2), composer border (2), session information (1).
	m.view.Height = max(1, m.height-m.input.Height()-5-m.approvalHeight()-m.terminalHeight()-m.previewHeight()-m.errorHeight()-m.questionHeight())
	if p := m.terminal(); p != nil && p.session != nil && m.terminalHeight() > 0 {
		p.session.Resize(m.width, m.terminalHeight()-2)
	}
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
	status, _, _ := m.composerStatus()
	box := m.approvalBox()
	if box != "" {
		box += "\n"
	}
	permission := ""
	if full := m.fullPermissionNotice(); full != "" {
		permission = " " + warning.Render(full)
	}
	context := muted.Render(m.contextStatus())
	quota := m.quotaStatus(time.Now(), max(1, width-ansi.StringWidth(permission)-ansi.StringWidth(context)-3))
	if quota != "" {
		quota += " "
	}
	quota += context
	info := permission + strings.Repeat(" ", max(1, width-1-ansi.StringWidth(permission)-ansi.StringWidth(quota))) + quota + " "
	track := ""
	if !m.view.AtBottom() {
		track = m.scrollTrack()
	}
	preview := ""
	if h := m.previewHeight(); h > 0 {
		rows := strings.Split(m.previewRows(max(1, width-4), h), "\n")
		for i := range rows {
			rows[i] = "  " + rows[i] + "  "
		}
		preview = strings.Join(rows, "\n") + "\n"
	}
	composer := m.input
	modal := m.errorFocused() || m.redactDialog != nil || m.terminalFocused() || m.panelFocus || m.report != nil || m.modelPicker != nil || m.restartConfirm != nil || m.questionDialog != nil || m.pasteDialog != nil || m.selectingTools() || (m.previewVisible() && m.filePreview.focused)
	if modal {
		composer.Blur()
	}
	conversation := m.restartOverlay(m.reportView(m.modelPickerOverlay(m.commandOverlay(m.conversationView()))))
	body := ""
	if question := m.questionPanel(); question != "" {
		// Question reserves its own rows. Nested paste previews can still use
		// the combined region without hiding the composer or losing its draft.
		body = m.redactOverlay(m.pasteOverlay(conversation+"\n"+track+"\n"+question)) + "\n"
	} else {
		body = m.redactOverlay(m.pasteOverlay(conversation)) + "\n" + track + "\n"
	}
	body += box + preview + strings.Repeat("\n", m.errorHeight()) + m.recordingStatusRow("  "+status, width) + "\n" +
		frame(m.decorateInputPastes(composer.View()), width, !modal && !m.focusList && !m.focusApproval && m.pathHints == nil) + "\n"
	if m.terminalHeight() > 0 {
		body += m.terminalView() + "\n"
	}
	body += clip(info, width)
	return screen(body, m.width, m.height)
}

func indentBlock(s string) string {
	if s == "" {
		return ""
	}
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

func insetRule(width int, style lipgloss.Style) string {
	if width <= 4 {
		return strings.Repeat(" ", max(0, width))
	}
	return "  " + style.Render(strings.Repeat("─", width-4)) + "  "
}
