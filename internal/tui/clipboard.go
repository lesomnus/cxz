package tui

import (
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// OSC 52 writes to the user's terminal clipboard, including over SSH. Never
// send display styling, line numbers added by the UI, or viewport truncation.
func (m *model) copyText(text string) {
	if m.cursorOutput == nil {
		m.showError("Clipboard output unavailable")
		return
	}
	if _, err := io.WriteString(m.cursorOutput, ansi.SetSystemClipboard(text)); err != nil {
		m.showError("Could not request clipboard copy: " + err.Error())
		return
	}
	m.notice = "Copy requested · requires terminal OSC 52 clipboard support"
}

func overlayButton(row string, x int, label string, hover bool, shade int) string {
	if x < 0 {
		return row
	}
	style := muted
	if hover {
		style = accent
		shade += 2
	}
	return ansi.Cut(row, 0, x) + indexedBackground(style.Render(label), shade) + ansi.Cut(row, x+ansi.StringWidth(label), ansi.StringWidth(row))
}

type transcriptSelection struct {
	session                    string
	offset, width              int
	startX, startY, endX, endY int
	dragging                   bool
	rows                       []string
	codeRows                   map[int]codeSelectionRow
}

type codeSelectionRow struct {
	start, end int
	omit       bool // The copy-button header and bottom padding have no source text.
}

func (m *model) selectionCodeRows(rows []string) map[int]codeSelectionRow {
	result := map[int]codeSelectionRow{}
	for _, b := range m.codeButtons {
		first := max(0, m.view.YOffset-b.y)
		last := min(b.contentRows+1, m.view.YOffset+len(rows)-1-b.y)
		for i := first; i <= last; i++ {
			row := b.y + i - m.view.YOffset
			probe := b
			probe.y += i
			if !m.codeButtonVisible(probe) {
				continue
			}
			end := min(b.contentX+b.contentWidth, ansi.StringWidth(strings.TrimRight(ansi.Strip(rows[row]), " ")))
			result[row] = codeSelectionRow{start: b.contentX, end: max(b.contentX, end), omit: i == 0 || i == b.contentRows+1}
		}
	}
	return result
}

func (m *model) selectionValid() bool {
	s, current := m.textSelection, m.current()
	return s != nil && current != nil && s.session == current.Id && s.offset == m.view.YOffset && s.width == m.width && !m.projectView && !m.accountView && m.previewInteraction() && m.settingsPage == nil && (m.memoryPage == nil && m.library == nil) && !m.terminalFocused()
}
func (s *transcriptSelection) bounds() (x1, y1, x2, y2 int) {
	x1, y1, x2, y2 = s.startX, s.startY, s.endX, s.endY
	if y1 > y2 || y1 == y2 && x1 > x2 {
		x1, y1, x2, y2 = x2, y2, x1, y1
	}
	return
}
func (s *transcriptSelection) columns(row int) (int, int) {
	x1, y1, x2, y2 := s.bounds()
	if row < y1 || row > y2 {
		return 0, 0
	}
	start, end := 0, ansi.StringWidth(s.rows[row])
	if row == y1 {
		start = x1
	}
	if row == y2 {
		end = x2
	}
	if code, ok := s.codeRows[row]; ok {
		if code.omit {
			return 0, 0
		}
		start = max(start, code.start)
		end = min(end, code.end)
	}
	return start, max(start, end)
}
func (s *transcriptSelection) text() string {
	_, y1, _, y2 := s.bounds()
	var rows []string
	for y := y1; y <= y2 && y < len(s.rows); y++ {
		if code, ok := s.codeRows[y]; ok && code.omit {
			continue
		}
		start, end := s.columns(y)
		rows = append(rows, strings.TrimRight(ansi.Strip(ansi.Cut(s.rows[y], start, end)), " "))
	}
	return strings.Join(rows, "\n")
}
func (m *model) selectionMouse(v tea.MouseMsg) bool {
	if s := m.textSelection; s != nil && s.dragging && m.selectionValid() {
		if v.Action == tea.MouseActionMotion || v.Action == tea.MouseActionRelease {
			s.endX = max(0, min(m.width, v.X-m.contentOffset()))
			s.endY = max(0, min(len(s.rows)-1, v.Y))
			if v.Action == tea.MouseActionRelease {
				s.dragging = false
			}
			return true
		}
	}
	return false
}
func (m *model) beginSelection(v tea.MouseMsg) {
	if v.Action != tea.MouseActionPress || v.Button != tea.MouseButtonLeft || m.current() == nil {
		return
	}
	m.textSelection = nil
	m.composerSelection = nil
	rows := strings.Split(m.conversationView(), "\n")
	if v.Y < 0 || v.Y >= len(rows) {
		return
	}
	m.textSelection = &transcriptSelection{session: m.current().Id, offset: m.view.YOffset, width: m.width, startX: v.X, endX: v.X, startY: v.Y, endY: v.Y, dragging: true, rows: rows, codeRows: m.selectionCodeRows(rows)}
}
func (m *model) selectionView(rows []string) {
	if !m.selectionValid() {
		return
	}
	s := m.textSelection
	// A streamed update or pinned prompt change invalidates the selection rather
	// than highlighting different text at the old coordinates.
	_, first, _, last := s.bounds()
	for i := first; i <= last; i++ {
		if i >= len(rows) || i >= len(s.rows) || ansi.Strip(rows[i]) != ansi.Strip(s.rows[i]) {
			m.textSelection = nil
			return
		}
	}
	for i := range rows {
		start, end := s.columns(i)
		if end > start {
			rows[i] = ansi.Cut(rows[i], 0, start) + indexedBackground(ansi.Cut(rows[i], start, end), 240) + ansi.Cut(rows[i], end, ansi.StringWidth(rows[i]))
		}
	}
}
func (m *model) copyFocusedText() {
	if m.errorFocused() {
		m.copyText(m.errorDialog.text)
		return
	}
	if m.workflow != nil || m.redactDialog != nil || m.questionFocused() || m.settingsPage != nil || (m.memoryPage != nil || m.library != nil) || m.accountView || m.projectView || m.panelFocus {
		return
	}
	switch {
	case m.composerAvailable() && !m.focusList && !m.focusApproval && m.selectedComposerText() != "":
		m.copyText(m.selectedComposerText())
	case m.previewVisible() && m.filePreview.focused && m.previewInteraction():
		m.copyText(m.filePreview.source)
	case m.report != nil:
		m.copyText(m.report.text)
	case m.selectionValid() && m.textSelection.text() != "" && m.previewInteraction():
		m.copyText(m.textSelection.text())
	default:
		m.notice = "Drag to select conversation text, or open a tool preview · Ctrl+C copy"
	}
}
