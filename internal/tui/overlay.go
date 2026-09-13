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
}

// A modal overlays existing rows without changing transcript height or scroll.
func overlayBox(view string, content []string, width int) string {
	rows := strings.Split(view, "\n")
	if width < 4 || len(rows) < 3 {
		return view
	}
	inner := width - 4
	if len(content) > len(rows)-2 {
		content = content[:len(rows)-2]
	}
	box := []string{teal.Render("╭" + strings.Repeat("─", width-2) + "╮")}
	for _, line := range content {
		line = clip(line, inner)
		box = append(box, teal.Render("│")+" "+line+strings.Repeat(" ", max(0, inner-ansi.StringWidth(line)))+" "+teal.Render("│"))
	}
	box = append(box, teal.Render("╰"+strings.Repeat("─", width-2)+"╯"))
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
	if strings.HasPrefix(p.title, "/help") {
		body = helpView(max(1, m.width-4), strings.TrimSpace(strings.TrimPrefix(p.title, "/help")))
	} else {
		body = ansi.Hardwrap(safeText(body), max(1, m.width-4), true)
	}
	lines := strings.Split(body, "\n")
	capacity := max(1, len(strings.Split(view, "\n"))-4)
	p.offset = max(0, min(p.offset, max(0, len(lines)-capacity)))
	content := []string{accent.Bold(true).Render(p.title)}
	content = append(content, lines[p.offset:min(len(lines), p.offset+capacity)]...)
	content = append(content, muted.Render("↑/↓ · PgUp/PgDn scroll · Esc close"))
	return overlayBox(view, content, m.width)
}

func (m *model) reportKey(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc", "ctrl+q":
		m.report = nil
	case "ctrl+c":
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
