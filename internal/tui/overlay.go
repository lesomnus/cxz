package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type reportOverlay struct {
	title, text, id, run string
	offset               int
	generation           uint64
	context              *contextReportView
}

// A modal overlays existing rows without changing transcript height or scroll.
func overlayBox(view string, content []string, width int, focused ...bool) string {
	rows := strings.Split(view, "\n")
	if width < 4 || len(rows) < 3 {
		return view
	}
	inner := width - 4
	if len(content) > len(rows)-2 {
		content = content[:len(rows)-2]
	}
	border := teal
	if len(focused) > 0 && focused[0] {
		border = accent
	}
	box := []string{border.Render("╭" + strings.Repeat("─", width-2) + "╮")}
	for _, line := range content {
		line = clip(line, inner)
		box = append(box, border.Render("│")+" "+line+strings.Repeat(" ", max(0, inner-ansi.StringWidth(line)))+" "+border.Render("│"))
	}
	box = append(box, border.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	copy(rows[len(rows)-len(box):], box)
	return strings.Join(rows, "\n")
}

func (m *model) openReport(title, text string) {
	id := ""
	run := ""
	if s := m.current(); s != nil {
		id = s.Id
		run = s.RunId
	}
	if p := m.modelPicker; p != nil {
		if p.cancel != nil {
			p.cancel()
		}
		m.modelPicker = nil
	}
	m.report = &reportOverlay{title: title, text: text, id: id, run: run}
}

func (m *model) reportView(view string) string {
	p := m.report
	if p == nil {
		return view
	}
	body := p.text
	if c := p.context; c != nil {
		if c.rawMode {
			body = c.raw
			if body == "" {
				body = "No raw response received yet."
			}
		} else {
			body = c.summary(max(1, m.width-4))
		}
	}
	if p.title == "/background" {
		body = m.backgroundReport()
	}
	if strings.HasPrefix(p.title, "/help") {
		body = helpView(max(1, m.width-4), strings.TrimSpace(strings.TrimPrefix(p.title, "/help")))
	} else {
		body = ansi.Hardwrap(safeText(body), max(1, m.width-4), true)
	}
	lines := strings.Split(body, "\n")
	capacity := max(1, len(strings.Split(view, "\n"))-4)
	p.offset = max(0, min(p.offset, max(0, len(lines)-capacity)))
	title := accent.Bold(true).Render(p.title)
	if c := p.context; c != nil {
		summary, raw := "[Summary]", "[Raw]"
		if c.rawMode {
			raw = accent.Bold(true).Render(raw)
		} else {
			summary = accent.Bold(true).Render(summary)
		}
		title = summary + " " + raw + " · Context"
	}
	content := []string{title}
	content = append(content, lines[p.offset:min(len(lines), p.offset+capacity)]...)
	footer := "↑/↓ · PgUp/PgDn scroll · Esc close"
	if p.title == "/logs" || p.title == "/logs project" {
		footer = "↑/↓ · PgUp/PgDn · Home/End · r refresh · Esc close"
		if m.width < 60 {
			footer = "↑↓ PgUp/Dn · r refresh · Esc"
		}
	}
	if p.context != nil {
		footer = "←/→ tabs · ↑↓ scroll · Esc close"
	}
	content = append(content, muted.Render(footer))
	if p.context != nil {
		p.context.headerY = len(strings.Split(view, "\n")) - min(len(content), len(strings.Split(view, "\n"))-2) - 1
	}
	return overlayBox(view, content, m.width, true)
}

func (m *model) reportKey(k tea.KeyMsg) tea.Cmd {
	if p := m.report; p.context != nil {
		switch k.String() {
		case "left", "s":
			p.contextTab(false)
			return nil
		case "right", "r":
			p.contextTab(true)
			return nil
		case "tab", "shift+tab":
			p.contextTab(!p.context.rawMode)
			return nil
		}
	}
	switch k.String() {
	case "r":
		if m.report.title == "/logs" || m.report.title == "/logs project" {
			return m.loadLogs()
		}
	case "esc", "ctrl+q":
		m.report = nil
	case "ctrl+d":
		return tea.Quit
	case "up":
		m.report.offset = max(0, m.report.offset-1)
	case "down":
		m.report.offset++
	case "pgup":
		m.report.offset = max(0, m.report.offset-max(1, m.view.Height-4))
	case "pgdown":
		m.report.offset += max(1, m.view.Height-4)
	case "home", "ctrl+home":
		m.report.offset = 0
	case "end", "ctrl+end":
		m.report.offset = int(^uint(0) >> 1)
	}
	return nil
}
