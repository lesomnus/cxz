package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

type codeButton struct {
	x, y   int // Column and absolute transcript row.
	source string
}

func (m *model) codeButtonVisible(b codeButton) bool {
	y := b.y - m.view.YOffset
	if y < 0 || y >= m.view.Height || m.historyShimmer != nil || m.historyOpening[m.watchID] {
		return false
	}
	if !m.selectingTools() && y < 2 {
		pinned := ""
		for _, span := range m.promptSpans {
			if span.end > m.view.YOffset {
				break
			}
			pinned = span.text
		}
		if pinned != "" {
			rows := strings.Split(ansi.Hardwrap(safeText(pinned), max(1, m.view.Width-2), true), "\n")
			if y < min(2, len(rows)) {
				return false
			}
		}
	}
	return true
}
func (m *model) codeBlockMouse(v tea.MouseMsg) bool {
	if !m.previewInteraction() || m.redactDialog != nil {
		return false
	}
	for _, b := range m.codeButtons {
		if !m.codeButtonVisible(b) || v.Y != b.y-m.view.YOffset || v.X < b.x || v.X >= b.x+3 {
			continue
		}
		m.codeHover = &b
		if v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress {
			m.textSelection = nil
			m.copyText(b.source)
		}
		return true
	}
	return false
}
func (m *model) codeButtonView(rows []string, pinned map[int]bool) {
	b := m.codeHover
	if b == nil || !m.codeButtonVisible(*b) {
		return
	}
	y := b.y - m.view.YOffset
	if y >= len(rows) || pinned[y] {
		return
	}
	// Ignore stale hover after a resize, session switch, or transcript refresh.
	for _, current := range m.codeButtons {
		if current == *b && ansi.StringWidth(rows[y]) >= b.x+3 {
			rows[y] = overlayButton(rows[y], b.x, " ⧉ ", true, 238)
			return
		}
	}
}
