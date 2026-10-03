package tui

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestComposerShiftSelectionEditing(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("hello 한글")
	m.input.CursorEnd()
	for range 2 {
		m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	}
	if got := m.selectedComposerText(); got != "한글" {
		t.Fatalf("selected %q", got)
	}
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	view := m.composerSelectionView(m.input.View())
	if !strings.Contains(view, "48;5;240") || !strings.Contains(ansi.Strip(view), "hello 한글") {
		t.Fatalf("selection not painted: %q", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("수정")})
	if got := m.input.Value(); got != "hello 수정" {
		t.Fatalf("replacement: %q", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := m.input.Value(); got != "hello 수" {
		t.Fatalf("deletion: %q", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if composerPosition(m.input) != 6 || m.selectedComposerText() != "" {
		t.Fatal("left did not collapse selection")
	}
}

func TestComposerMouseCursorMapping(t *testing.T) {
	m := conversationModel()
	m.width = 28
	m.input.SetValue("한글 hello world words wrap here\nsecond line\nthird\nfourth\nfifth\nsixth\nlast")
	m.resize()
	// Compare hit testing against the textarea's rendered cursor, including its
	// private scroll offset, soft wraps, CJK widths and the sidebar offset.
	for _, pos := range []int{0, 1, 2, 5, 12, 20, 30, 38, 55, 65} {
		m.setComposerPosition(pos)
		m.resize()
		probe := m.input
		probe.Cursor.Blink = false
		probe.Cursor.Style = cursorProbeStyle
		x, y, ok := widgetCursor(probe.View())
		if !ok {
			t.Fatal("cursor unavailable")
		}
		got := m.composerPoint(x-2, y)
		if got != composerPosition(m.input) {
			t.Fatalf("position %d at (%d,%d): hit %d want %d", pos, x, y, got, composerPosition(m.input))
		}
		top := m.height - m.input.Height() - 2 - m.terminalHeight()
		// Move away on the same viewport, then click the recorded location.
		m.composerSelection = nil
		event := tea.MouseMsg{X: m.contentOffset() + 1 + x, Y: top + y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
		m.Update(event)
		if composerPosition(m.input) != got {
			t.Fatalf("click moved to %d want %d", composerPosition(m.input), got)
		}
		event.Action = tea.MouseActionRelease
		m.Update(event)
	}
}

func TestComposerMouseDragAndReplace(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("one two three")
	m.resize()
	top := m.height - m.input.Height() - 2 - m.terminalHeight()
	left := m.contentOffset() + 3
	m.Update(tea.MouseMsg{X: left + 4, Y: top, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.MouseMsg{X: left + 7, Y: top, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m.Update(tea.MouseMsg{X: left + 7, Y: top, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	if got := m.selectedComposerText(); got != "two" {
		t.Fatalf("drag selected %q", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("changed"), Paste: true})
	if m.input.Value() != "one changed three" {
		t.Fatalf("paste replacement: %q", m.input.Value())
	}
}

func TestComposerSelectionMultilineAndStale(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("first\nsecond")
	m.resize()
	m.setComposerPosition(2)
	m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	if got := m.selectedComposerText(); got != "rst\nse" {
		t.Fatalf("down selection %q", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	if m.selectedComposerText() != "" {
		t.Fatal("reverse selection did not shrink")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	m.input.SetValue("different")
	if m.selectedComposerText() != "" {
		t.Fatal("selection survived external draft replacement")
	}
	m.input.CursorEnd()
	m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	m.sessions[0].Id = "other"
	if m.selectedComposerText() != "" {
		t.Fatal("selection leaked across sessions")
	}
}

func TestComposerSelectionDeletesWholeChip(t *testing.T) {
	m := conversationModel()
	token := "[Paste fixture]"
	m.pastes = map[string]*pastedText{token: {token: token, body: "private body"}}
	m.input.SetValue("a " + token + " z")
	m.setComposerPosition(5)
	m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	if m.selectedComposerText() != token {
		t.Fatalf("partial chip: %q", m.selectedComposerText())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if m.input.Value() != "a  z" {
		t.Fatalf("split chip: %q", m.input.Value())
	}
}

func TestComposerSelectionCopyCutAndEscape(t *testing.T) {
	m := conversationModel()
	var out bytes.Buffer
	m.cursorOutput = &cursorWriter{out: &out}
	m.input.SetValue("한글 copy")
	for range 4 {
		m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !strings.Contains(out.String(), ansi.SetSystemClipboard("copy")) || m.input.Value() != "한글 copy" {
		t.Fatal("copy changed input or copied wrong text")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.selectedComposerText() != "" || m.interruptKey != "" {
		t.Fatal("escape interrupted instead of deselecting")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if m.input.Value() != "한글 opy" {
		t.Fatalf("cut: %q", m.input.Value())
	}
}

func TestComposerSelectionGraphemesAndModal(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("A👩‍💻é")
	m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	if m.selectedComposerText() != "é" {
		t.Fatal("split combining grapheme")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	if m.selectedComposerText() != "👩‍💻é" {
		t.Fatal("split emoji grapheme")
	}
	m.composerSelection = nil
	m.panelFocus = true
	m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	if m.composerSelection != nil {
		t.Fatal("sidebar selected composer")
	}
	m.panelFocus = false
	m.settingsPage = &settingsPage{}
	if m.composerMouse(tea.MouseMsg{X: m.contentOffset() + 3, Y: m.height - m.input.Height() - 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}) {
		t.Fatal("mouse reached composer through settings")
	}
}

// Word-wise selection has to agree with word-wise motion: whatever ctrl+left
// walks over is what ctrl+shift+left takes.
func TestComposerWordSelection(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("alpha beta gamma")
	m.resize()
	m.input.CursorEnd()
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlShiftLeft})
	if got := m.selectedComposerText(); got != "gamma" {
		t.Fatalf("one word back selected %q", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlShiftLeft})
	if got := m.selectedComposerText(); got != "beta gamma" {
		t.Fatalf("two words back selected %q", got)
	}
	// Coming back the other way shrinks the same selection rather than starting
	// a new one in the other direction.
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlShiftRight})
	if got := m.selectedComposerText(); got != " gamma" {
		t.Fatalf("forward shrink selected %q", got)
	}
	// A plain word move is still a move, and it drops the selection.
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlLeft})
	if m.selectedComposerText() != "" {
		t.Fatal("word motion kept a selection")
	}
}

// Home and End address the row on screen. A wrapped draft is one line in the
// value and several rows in the view, so walking it needs the view's idea of an
// edge, and a second press has to move rather than sit there.
func TestComposerHomeEndFollowVisibleRows(t *testing.T) {
	m := conversationModel()
	// Long enough to wrap whatever width the composer settles on, and a second
	// line behind it so a row edge is not also a line edge.
	m.input.SetValue(strings.Repeat("alpha beta gamma delta ", 8) + "zeta\nsecond line here")
	m.resize()
	rows := m.composerRows()
	if len(rows) < 4 || rows[1].line != 0 {
		t.Fatal("fixture did not wrap", rows)
	}
	press := func(k tea.KeyType) int {
		m.Update(tea.KeyMsg{Type: k})
		return composerPosition(m.input)
	}
	m.setComposerPosition(rows[0].start + 10)
	if got := press(tea.KeyHome); got != rows[0].start {
		t.Fatal("home left the row start", got)
	}
	// The first row ends mid line, so its last position is the one that still
	// draws on it: the position after it is where the next row begins.
	if got := press(tea.KeyEnd); got != rows[0].end-1 {
		t.Fatal("end did not stop at the visible row end", got)
	}
	if got := press(tea.KeyEnd); got != rows[1].end-1 {
		t.Fatal("end again did not step to the next row", got)
	}
	if got := press(tea.KeyHome); got != rows[1].start {
		t.Fatal("home did not return to this row's start", got)
	}
	if got := press(tea.KeyHome); got != rows[0].start {
		t.Fatal("home again did not step to the previous row", got)
	}
	if got := press(tea.KeyHome); got != rows[0].start {
		t.Fatal("home ran off the front of the draft", got)
	}
	// The last row of a line owns the position after its last character: no row
	// starts there, so the cursor still draws on it.
	last := rows[len(rows)-1]
	m.setComposerPosition(last.start)
	if got := press(tea.KeyEnd); got != last.end {
		t.Fatal("end short of the final row", got)
	}
	if got := press(tea.KeyEnd); got != last.end {
		t.Fatal("end ran off the back of the draft", got)
	}
	// An edge key is a move, so it collapses a selection like the arrows do.
	m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	if m.selectedComposerText() == "" {
		t.Fatal("no selection to collapse")
	}
	press(tea.KeyHome)
	if m.selectedComposerText() != "" {
		t.Fatal("home kept a selection")
	}
}

// The composer stops growing at six rows, so a longer draft continues off
// screen. The bar is the only thing that says so, and where it sits has to
// agree with the rows the widget is actually showing.
func TestComposerScrollbarTracksTheDraft(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	m := conversationModel()
	m.input.SetValue("one\ntwo\nthree")
	m.resize()
	for _, row := range strings.Split(m.composerScrollbar(m.input.View()), "\n") {
		if strings.Contains(row, "│") {
			t.Fatal("a draft that fits drew a scrollbar:", row)
		}
	}
	m.input.SetValue(strings.Repeat("a line of draft\n", 12) + "last")
	m.resize()
	thumb, track := muted.Render("│"), zeroStyle.Render("│")
	bar := func() []bool {
		var out []bool
		for _, row := range strings.Split(m.composerScrollbar(m.input.View()), "\n") {
			switch {
			case strings.HasSuffix(row, thumb):
				out = append(out, true)
			case strings.HasSuffix(row, track):
				out = append(out, false)
			default:
				t.Fatal("row carries no scrollbar cell:", row)
			}
			if w := ansi.StringWidth(row); w != m.width-2 {
				t.Fatal("scrollbar row is", w, "cells, want", m.width-2)
			}
		}
		return out
	}
	top := bar()
	if len(top) != m.input.Height() || !top[0] || top[len(top)-1] {
		t.Fatal("thumb is not at the top of an unscrolled draft:", top)
	}
	m.setComposerPosition(len([]rune(m.input.Value())))
	m.resize()
	end := bar()
	if end[0] || !end[len(end)-1] {
		t.Fatal("thumb is not at the bottom of a draft scrolled to its end:", end)
	}
	// The thumb is a proportion of the draft, not a single cell: six rows of
	// thirteen is about half the bar.
	size := 0
	for _, on := range end {
		if on {
			size++
		}
	}
	if size < 2 || size >= len(end) {
		t.Fatal("thumb covers", size, "of", len(end), "rows")
	}
}

// The wheel over the composer moves through the draft. The widget's view follows
// its cursor, so the cursor is what moves, and everything that reads a position
// -- the scrollbar, the selection, the cursor itself -- stays in agreement.
func TestComposerWheelScrollsTheDraft(t *testing.T) {
	m := conversationModel()
	m.input.SetValue(strings.Repeat("a line of draft\n", 12) + "last")
	m.resize()
	m.setComposerPosition(5)
	m.resize()
	top := m.height - m.input.Height() - 2 - m.terminalHeight()
	x := m.contentOffset() + 4
	wheel := func(y int, button tea.MouseButton) bool {
		return m.composerMouse(tea.MouseMsg{X: x, Y: y, Button: button, Action: tea.MouseActionPress})
	}
	row := func() int { return composerRowIndex(m.composerRows(), composerPosition(m.input)) }
	if !wheel(top+1, tea.MouseButtonWheelDown) {
		t.Fatal("the composer did not take the wheel")
	}
	if row() != 3 {
		t.Fatal("a notch moved to row", row(), "want three rows down")
	}
	// The column is held, the way an arrow key holds it.
	rows := m.composerRows()
	if got := composerPosition(m.input) - rows[row()].start; got != 5 {
		t.Fatal("column became", got, "want 5")
	}
	// The view follows, which is the point of scrolling at all.
	before := m.composerScroll(rows)
	for range 3 {
		wheel(top+1, tea.MouseButtonWheelDown)
	}
	if m.composerScroll(m.composerRows()) <= before {
		t.Fatal("the visible rows did not move")
	}
	// Both ends stop, and say so rather than swallowing the event.
	for range 10 {
		wheel(top+1, tea.MouseButtonWheelDown)
	}
	if wheel(top+1, tea.MouseButtonWheelDown) {
		t.Fatal("scrolled past the end of the draft")
	}
	for range 10 {
		wheel(top+1, tea.MouseButtonWheelUp)
	}
	if wheel(top+1, tea.MouseButtonWheelUp) || row() != 0 {
		t.Fatal("scrolled past the start of the draft, at row", row())
	}
	// Outside the composer it is someone else's event, and a draft that fits has
	// nothing to scroll.
	if wheel(0, tea.MouseButtonWheelDown) {
		t.Fatal("took a wheel event from the conversation")
	}
	m.input.SetValue("short")
	m.resize()
	if wheel(m.height-m.input.Height()-1, tea.MouseButtonWheelDown) {
		t.Fatal("scrolled a draft that fits")
	}
}

// The bar's column is reserved whether or not a bar is in it. Taking the cell
// only once the draft outgrows the composer rewraps the text on the keystroke
// that caused it, which reads as the editor stumbling.
func TestComposerReservesTheScrollbarColumn(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	m := conversationModel()
	line := "a line of draft " + strings.Repeat("x", 50)
	measure := func(n int) (int, int, bool) {
		m.input.SetValue(strings.Repeat(line+"\n", n-1) + line)
		m.resize()
		rows := strings.Split(m.composerScrollbar(m.input.View()), "\n")
		width := 0
		for _, row := range rows {
			if w := ansi.StringWidth(row); w > width {
				width = w
			}
		}
		bar := strings.HasSuffix(rows[0], muted.Render("│")) || strings.HasSuffix(rows[0], zeroStyle.Render("│"))
		return m.input.Width(), width, bar
	}
	// Five rows fit; seven do not, so the bar appears between these two.
	fitsText, fitsRow, fitsBar := measure(5)
	overText, overRow, overBar := measure(7)
	if fitsBar || !overBar {
		t.Fatal("the bar did not appear exactly when the draft outgrew the composer", fitsBar, overBar)
	}
	if fitsText != overText {
		t.Fatal("the text width changed with the bar:", fitsText, "then", overText)
	}
	if fitsRow != overRow {
		t.Fatal("the rendered width changed with the bar:", fitsRow, "then", overRow)
	}
	// The reserved cell is inside the frame's content, not beyond it.
	if overRow != m.width-2 {
		t.Fatal("rows are", overRow, "cells, want", m.width-2)
	}
	// Wrapping is computed against the same width the widget is given, so the
	// row count the composer reports matches what it draws.
	if got := len(m.composerRows()); got != 7 {
		t.Fatal("draft of seven lines wrapped into", got, "rows")
	}
}
