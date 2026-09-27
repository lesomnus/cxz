package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"time"
)

const toolDoubleClickInterval = 400 * time.Millisecond

type toolClick struct {
	session      string
	seq          uint64
	x, y, offset int
	at           time.Time
}

func (m *model) toolAtRow(y int) uint64 {
	if y < 0 || y >= m.view.Height || m.current() == nil || !m.previewInteraction() {
		return 0
	}
	// The pinned prompt replaces the first one or two visible history rows.
	if y < 2 && !m.selectingTools() {
		for _, span := range m.promptSpans {
			if span.end <= m.view.YOffset {
				return 0
			}
		}
	}
	return m.toolRows[m.view.YOffset+y]
}
func (m *model) toolMouse(v tea.MouseMsg, now time.Time) bool {
	seq := m.toolAtRow(v.Y)
	if v.Action != tea.MouseActionPress || v.Button != tea.MouseButtonLeft {
		return false
	}
	if seq == 0 {
		m.toolClick = nil
		return false
	}
	s := m.current().Id
	if last := m.toolClick; last != nil && last.session == s && last.seq == seq && last.x == v.X && last.y == v.Y && last.offset == m.view.YOffset && now.Sub(last.at) >= 0 && now.Sub(last.at) <= toolDoubleClickInterval {
		m.toolClick = nil
		m.textSelection = nil
		return m.openPreviewSequence(seq)
	}
	m.toolClick = &toolClick{session: s, seq: seq, x: v.X, y: v.Y, offset: m.view.YOffset, at: now}
	// Let the first click start the existing drag-to-select interaction.
	return false
}
func (m *model) toolHoverView(rows []string, obscured map[int]bool) {
	h := m.toolHover
	s := m.current()
	if h == nil || s == nil || h.session != s.Id || !m.previewInteraction() || m.projectView || m.accountView {
		return
	}
	for row := range rows {
		if !obscured[row] && m.toolRows[m.view.YOffset+row] == h.seq {
			rows[row] = indexedBackground(rows[row], 236)
		}
	}
}
