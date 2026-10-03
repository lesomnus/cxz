package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

type composerSelection struct {
	value, session string
	anchor, head   int
	dragging       bool
}
type composerRow struct{ start, end, line, column int }
type composerLayout struct {
	value string
	width int
	rows  []composerRow
}

func composerPosition(in textarea.Model) int {
	pos := 0
	lines := strings.Split(in.Value(), "\n")
	for _, line := range lines[:in.Line()] {
		pos += utf8.RuneCountInString(line) + 1
	}
	li := in.LineInfo()
	return pos + li.StartColumn + li.ColumnOffset
}
func (m *model) setComposerPosition(pos int) {
	r := []rune(m.input.Value())
	pos = max(0, min(len(r), pos))
	prefix := string(r[:pos])
	line := strings.Count(prefix, "\n")
	parts := strings.Split(prefix, "\n")
	column := utf8.RuneCountInString(parts[len(parts)-1])
	for m.input.Line() > line {
		m.input.CursorStart()
		m.input.CursorUp()
	}
	for m.input.Line() < line {
		m.input.CursorEnd()
		m.input.CursorDown()
	}
	m.input.SetCursor(column)
}

func (m *model) composerAvailable() bool {
	return !m.projectView && !m.accountView && !m.creating && !m.renaming && m.settingsPage == nil && m.library == nil && m.memoryPage == nil && m.workflow == nil && m.pasteDialog == nil && m.redactDialog == nil && m.restartConfirm == nil && m.report == nil && m.modelPicker == nil && m.sessionArchive == nil && !m.errorFocused() && !m.questionFocused() && !m.terminalFocused() && !m.selectingTools() && !(m.previewVisible() && m.filePreview.focused)
}
func (m *model) composerSession() string {
	if s := m.current(); s != nil {
		return s.Id
	}
	return ""
}
func (m *model) composerSelectionValid() bool {
	s := m.composerSelection
	if s == nil {
		return false
	}
	if s.value != m.input.Value() || s.session != m.composerSession() || s.head != composerPosition(m.input) {
		m.composerSelection = nil
		return false
	}
	return true
}
func (m *model) composerBounds() (int, int) {
	if !m.composerSelectionValid() {
		return 0, 0
	}
	s := m.composerSelection
	a, b := min(s.anchor, s.head), max(s.anchor, s.head)
	if a == b {
		return a, b
	}
	// A chip is one editable object even if its label wraps across rows.
	for token := range m.pastes {
		for offset := 0; offset < len(s.value); {
			i := strings.Index(s.value[offset:], token)
			if i < 0 {
				break
			}
			i += offset
			start := utf8.RuneCountInString(s.value[:i])
			end := start + utf8.RuneCountInString(token)
			if a < end && b > start {
				a = min(a, start)
				b = max(b, end)
			}
			offset = i + len(token)
		}
	}
	return a, b
}
func (m *model) selectedComposerText() string {
	a, b := m.composerBounds()
	if a == b {
		return ""
	}
	return string([]rune(m.input.Value())[a:b])
}
func (m *model) deleteComposerSelection() bool {
	a, b := m.composerBounds()
	if a == b {
		return false
	}
	r := []rune(m.input.Value())
	m.composerSelection = nil
	m.pasteSelection = nil
	m.setPathInput(string(r[:a])+string(r[b:]), a)
	return true
}

// Home and End work on the row you can see rather than the line behind it: a
// wrapped draft is one line in the value and several rows on screen, and a key
// that jumps past what is visible is a key you cannot aim with. Pressing again
// on an edge steps to the neighbouring row's edge, so the pair walks the draft
// instead of doing nothing on the second press.
func (m *model) composerRowEdge(forward bool) {
	rows := m.composerRows()
	if len(rows) == 0 {
		return
	}
	pos := composerPosition(m.input)
	i := composerRowIndex(rows, pos)
	target := rows[i].start
	if forward {
		target = composerRowEnd(rows, i)
	}
	if target == pos {
		switch {
		case forward && i+1 < len(rows):
			target = composerRowEnd(rows, i+1)
		case !forward && i > 0:
			target = rows[i-1].start
		}
	}
	m.setComposerPosition(target)
}

// composerRowIndex answers the question the widget answers when it draws: which
// row does this position appear on. A position on a wrap boundary belongs to the
// row that starts there, and one on a line's trailing newline to the row that
// ends there.
func composerRowIndex(rows []composerRow, pos int) int {
	for i, row := range rows {
		if pos < row.end {
			return i
		}
		if i+1 < len(rows) && pos < rows[i+1].start {
			return i
		}
	}
	return len(rows) - 1
}

// composerRowEnd is the last position that still renders on the row. A soft wrap
// has no character of its own, so the position after a wrapped row's last
// character is also the position before the next row's first, and the widget
// draws it there; stopping one short keeps End on the row it was pressed on.
func composerRowEnd(rows []composerRow, i int) int {
	row := rows[i]
	if i+1 < len(rows) && rows[i+1].line == row.line {
		return max(row.start, row.end-1)
	}
	return row.end
}

func composerGraphemeMove(value string, pos int, forward bool) int {
	g := uniseg.NewGraphemes(value)
	start := 0
	for g.Next() {
		end := start + utf8.RuneCountInString(g.Str())
		if forward && pos < end {
			return end
		}
		if !forward && pos <= end {
			return start
		}
		start = end
	}
	return start
}

// extendComposerSelection is a move with the anchor kept. The move runs through
// the same code the unshifted key would use, so a selection can never cover
// something the cursor could not have reached by itself.
func (m *model) extendComposerSelection(move func()) {
	if !m.composerSelectionValid() {
		m.composerSelection = &composerSelection{value: m.input.Value(), session: m.composerSession(), anchor: composerPosition(m.input)}
	}
	s := m.composerSelection
	m.pasteSelection = nil
	move()
	// Bounds expand partial chip selections to whole objects.
	s.head = composerPosition(m.input)
	s.dragging = false
	m.resize()
}

func (m *model) composerKey(k tea.KeyMsg) (bool, tea.Cmd) {
	if !m.composerAvailable() || m.panelFocus || m.focusList || m.focusApproval {
		return false, nil
	}
	// Word-wise selection reuses the widget's own word motion, so what
	// ctrl+left selects is exactly what ctrl+left would have walked over.
	moves := map[string]tea.KeyType{
		"shift+left": tea.KeyLeft, "shift+right": tea.KeyRight, "shift+up": tea.KeyUp, "shift+down": tea.KeyDown,
		"ctrl+shift+left": tea.KeyCtrlLeft, "ctrl+shift+right": tea.KeyCtrlRight,
	}
	if direction, ok := moves[k.String()]; ok && !k.Paste {
		m.extendComposerSelection(func() {
			if direction == tea.KeyLeft || direction == tea.KeyRight {
				m.setComposerPosition(composerGraphemeMove(m.input.Value(), composerPosition(m.input), direction == tea.KeyRight))
			} else {
				m.input, _ = m.input.Update(tea.KeyMsg{Type: direction})
			}
		})
		return true, nil
	}
	// Shift turns the edge keys into a selection to the same place they move to,
	// including the second press that steps to the neighbouring row.
	if forward, ok := map[string]bool{"shift+home": false, "shift+end": true}[k.String()]; ok && !k.Paste {
		m.extendComposerSelection(func() { m.composerRowEdge(forward) })
		return true, nil
	}
	if edge := k.String(); (edge == "home" || edge == "end") && !k.Paste {
		m.composerSelection = nil
		m.pasteSelection = nil
		m.composerRowEdge(edge == "end")
		// A chip's label can wrap, so a row edge can land inside one object.
		m.snapChipCursor()
		m.resize()
		return true, nil
	}
	if !m.composerSelectionValid() {
		return false, nil
	}
	a, b := m.composerBounds()
	if a == b {
		m.composerSelection = nil
		return false, nil
	}
	switch k.String() {
	case "left", "right":
		if !k.Paste {
			pos := a
			if k.Type == tea.KeyRight {
				pos = b
			}
			m.composerSelection = nil
			m.setComposerPosition(pos)
			m.resize()
			return true, nil
		}
	case "backspace", "delete":
		if !k.Paste {
			m.deleteComposerSelection()
			return true, nil
		}
	case "ctrl+x":
		if !k.Paste {
			m.copyText(m.selectedComposerText())
			m.deleteComposerSelection()
			return true, nil
		}
	}
	if k.Type == tea.KeyRunes || k.Type == tea.KeySpace || k.Type == tea.KeyEnter || k.String() == "alt+enter" || k.String() == "ctrl+j" || k.Paste {
		m.deleteComposerSelection()
	} else {
		m.composerSelection = nil
	}
	return false, nil
}

// Use textarea's own wrapping metadata rather than assuming hard wrapping.
// The cache contains only offsets into the draft and is invalidated on resize/edit.
func (m *model) composerRows() []composerRow {
	value := m.input.Value()
	if c := m.composerLayout; c != nil && c.value == value && c.width == m.input.Width() {
		return c.rows
	}
	var rows []composerRow
	base := 0
	for line, text := range strings.Split(value, "\n") {
		probe := m.input
		probe.SetValue(text)
		length := utf8.RuneCountInString(text)
		for start := 0; start <= length; {
			probe.SetCursor(start)
			li := probe.LineInfo()
			end := min(length, li.StartColumn+li.Width)
			rows = append(rows, composerRow{base + li.StartColumn, base + end, line, li.StartColumn})
			if li.RowOffset+1 >= li.Height || li.Width == 0 {
				break
			}
			start = li.StartColumn + li.Width
		}
		base += length + 1
	}
	m.composerLayout = &composerLayout{value, m.input.Width(), rows}
	return rows
}
func (m *model) composerScroll(rows []composerRow) int {
	probe := m.input
	probe.Focus()
	probe.Cursor.Blink = false
	probe.Cursor.Style = cursorProbeStyle
	_, visible, ok := widgetCursor(probe.View())
	if !ok {
		return 0
	}
	li := m.input.LineInfo()
	for i, row := range rows {
		if row.line == m.input.Line() && row.column == li.StartColumn {
			return max(0, i-visible)
		}
	}
	return 0
}
func (m *model) composerPoint(x, y int) int {
	rows := m.composerRows()
	offset := m.composerScroll(rows)
	if y+offset >= len(rows) {
		return utf8.RuneCountInString(m.input.Value())
	}
	row := rows[max(0, y+offset)]
	r := []rune(m.input.Value())
	pos := row.start
	col := 0
	g := uniseg.NewGraphemes(string(r[row.start:row.end]))
	for g.Next() {
		w := g.Width()
		if x < col+w {
			break
		}
		col += w
		pos += utf8.RuneCountInString(g.Str())
	}
	return pos
}
func (m *model) composerMouse(v tea.MouseMsg) bool {
	if !m.composerAvailable() {
		return false
	}
	top := m.height - m.input.Height() - 2 - m.terminalHeight()
	x, y := v.X-m.contentOffset()-3, v.Y-top
	inside := y >= 0 && y < m.input.Height() && v.X >= m.contentOffset()+1 && v.X < m.contentOffset()+m.width-1
	if v.Button == tea.MouseButtonWheelUp || v.Button == tea.MouseButtonWheelDown {
		return inside && m.scrollComposer(v.Button == tea.MouseButtonWheelDown)
	}
	dragging := m.composerSelectionValid() && m.composerSelection.dragging
	if dragging && (v.Action == tea.MouseActionMotion || v.Action == tea.MouseActionRelease) {
		s := m.composerSelection
		pos := m.composerPoint(max(0, x), y)
		m.setComposerPosition(pos)
		s.head = composerPosition(m.input)
		m.resize()
		if v.Action == tea.MouseActionRelease {
			s.dragging = false
		}
		return true
	}
	if v.Action != tea.MouseActionPress || v.Button != tea.MouseButtonLeft || y < 0 || y >= m.input.Height() || v.X < m.contentOffset()+1 || v.X >= m.contentOffset()+m.width-1 {
		return false
	}
	pos := m.composerPoint(max(0, x), y)
	m.panelFocus, m.focusList, m.focusApproval = false, false, false
	m.input.Focus()
	m.textSelection = nil
	m.pasteSelection = nil
	m.setComposerPosition(pos)
	pos = composerPosition(m.input)
	m.composerSelection = &composerSelection{value: m.input.Value(), session: m.composerSession(), anchor: pos, head: pos, dragging: true}
	m.resize()
	return true
}
func (m *model) composerSelectionView(view string) string {
	if !m.composerAvailable() {
		return view
	}
	a, b := m.composerBounds()
	if a == b {
		return view
	}
	layout := m.composerRows()
	offset := m.composerScroll(layout)
	r := []rune(m.input.Value())
	rows := strings.Split(view, "\n")
	for y := range rows {
		if y+offset >= len(layout) {
			break
		}
		row := layout[y+offset]
		start, end := max(a, row.start), min(b, row.end)
		newline := b > row.end && a <= row.end && row.end < len(r) && r[row.end] == '\n'
		if start >= end && !newline {
			continue
		}
		if start > end {
			start = end
		}
		left := 2 + ansi.StringWidth(string(r[row.start:start]))
		right := 2 + ansi.StringWidth(string(r[row.start:end]))
		if newline {
			right++
		}
		rows[y] = ansi.Cut(rows[y], 0, left) + indexedBackground(ansi.Cut(rows[y], left, right), 240) + ansi.Cut(rows[y], right, ansi.StringWidth(rows[y]))
	}
	return strings.Join(rows, "\n")
}

// scrollComposer moves the view by rows under the wheel. The widget has no
// scroll of its own -- its view follows the cursor -- so the cursor is what
// moves, which also keeps the scrollbar, the selection and the cursor reading
// one position rather than three. A draft that fits has nothing to scroll, and
// says so, rather than swallowing the event.
func (m *model) scrollComposer(down bool) bool {
	rows := m.composerRows()
	if len(rows) <= m.input.Height() {
		return false
	}
	step := -composerWheelRows
	if down {
		step = composerWheelRows
	}
	pos := composerPosition(m.input)
	from := composerRowIndex(rows, pos)
	to := max(0, min(len(rows)-1, from+step))
	if to == from {
		return false
	}
	m.composerSelection = nil
	m.pasteSelection = nil
	// Hold the column where there is one to hold, the way an arrow key does.
	m.setComposerPosition(min(rows[to].start+pos-rows[from].start, composerRowEnd(rows, to)))
	m.snapChipCursor()
	m.resize()
	return true
}

const composerWheelRows = 3

// composerScrollbar marks where the draft is when it stops fitting. The composer
// grows to maxComposerRows and then holds, so past that the rest of a long
// message is off screen with nothing on screen to say so. The bar reads the same
// layout the selection and the cursor read, so it cannot disagree with them
// about where the draft is.
func (m *model) composerScrollbar(view string) string {
	rows := strings.Split(view, "\n")
	layout := m.composerRows()
	if len(rows) == 0 || m.width < 4 {
		return view
	}
	// The column is always there; a bar is in it only when the draft runs past
	// what the composer shows. Reserving it means the text never rewraps as the
	// bar arrives, which is what made adding a line feel like a stutter.
	start, size := 0, 0
	if len(layout) > len(rows) {
		offset := min(m.composerScroll(layout), len(layout)-len(rows))
		visible, total := len(rows), len(layout)
		size = max(1, visible*visible/total)
		span := max(1, total-visible)
		start = min(visible-size, (offset*(visible-size)+span/2)/span)
	}
	width := m.width - 2 - composerBarCells
	for y, row := range rows {
		bar := " "
		switch {
		case size == 0:
		case y >= start && y < start+size:
			bar = muted.Render("│")
		default:
			bar = zeroStyle.Render("│")
		}
		text := ansi.Cut(row, 0, width)
		rows[y] = text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text))) + bar
	}
	return strings.Join(rows, "\n")
}

// composerBarCells is the width the scrollbar column holds, reserved at all
// times so the draft's wrapping does not depend on whether it is scrollable.
const composerBarCells = 1

// maxComposerRows is how far the composer grows before it scrolls instead. The
// conversation keeps the rest of the screen, so a short terminal holds the
// composer to a third of it and reaches this only when there is room.
const maxComposerRows = 12
