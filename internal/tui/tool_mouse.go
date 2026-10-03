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
	// The pinned prompt replaces the first history rows it needs, and a tool
	// underneath one of them is not the thing being pointed at.
	if _, rows := m.pinnedPrompt(m.view.Height); y < rows {
		return 0
	}
	return m.toolRows[m.view.YOffset+y]
}

// pinnedPromptMouse sends the view back to the input pinned at the top. Those
// rows are a signpost saying which message you are reading the answer to;
// clicking a signpost should take you to what it names.
func (m *model) pinnedPromptMouse(v tea.MouseMsg) bool {
	if v.Action != tea.MouseActionPress || v.Button != tea.MouseButtonLeft || m.current() == nil {
		return false
	}
	span, rows := m.pinnedPrompt(m.view.Height)
	if rows == 0 || v.Y < 0 || v.Y >= rows {
		return false
	}
	m.textSelection = nil
	m.toolClick = nil
	m.view.SetYOffset(max(0, span.start))
	return true
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
