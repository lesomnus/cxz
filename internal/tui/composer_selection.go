package tui

import (
	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/bed"
)

type composerRow = bed.Row

const composerBarCells = 1
const maxComposerRows = 12
const composerWheelRows = 3

func composerPosition(in bed.Model) int { return bed.Position(in) }
func (m *model) prepareEditor() {
	m.input.DocumentKey = m.composerSession()
	m.input.AtomicTokens = m.input.AtomicTokens[:0]
	for token := range m.pastes {
		m.input.AtomicTokens = append(m.input.AtomicTokens, token)
	}
}
func (m *model) setComposerPosition(pos int)  { m.input.SetPosition(pos) }
func (m *model) composerSelectionValid() bool { m.prepareEditor(); return m.input.SelectionValid() }
func (m *model) composerBounds() (int, int)   { m.prepareEditor(); return m.input.SelectionBounds() }
func (m *model) selectedComposerText() string { m.prepareEditor(); return m.input.SelectedText() }
func (m *model) deleteComposerSelection() bool {
	m.prepareEditor()
	if !m.input.DeleteSelection() {
		return false
	}
	m.pasteSelection = nil
	m.resize()
	return true
}
func (m *model) composerRowEdge(forward bool) { m.input.RowEdge(forward) }
func composerGraphemeMove(value string, pos int, forward bool) int {
	return bed.GraphemeMove(value, pos, forward)
}
func (m *model) extendComposerSelection(move func()) {
	m.prepareEditor()
	m.pasteSelection = nil
	m.input.ExtendSelection(move)
	m.resize()
}
func (m *model) composerKey(k tea.KeyMsg) (bool, tea.Cmd) {
	if !m.composerAvailable() || m.panelFocus || m.focusList || m.focusApproval {
		return false, nil
	}
	// Large pastes must pass through cxz's chip capture before bed broadcasts.
	if k.Paste && m.multiComposer() && m.capturePaste(k) {
		return true, nil
	}
	if !k.Paste && (k.String() == "tab" || k.String() == "shift+tab") {
		return false, nil
	}
	m.prepareEditor()
	before := m.input.Value()
	hadSelection := m.input.SelectionValid()
	// These keys belong to the product, not textarea. In particular Ctrl+D
	// must detach, not delete at every cursor through the base textarea binding.
	if !k.Paste {
		switch k.String() {
		case "ctrl+d", "ctrl+s", "ctrl+home", "ctrl+end", "pgup", "pgdown", "ctrl+q", "ctrl+n", "ctrl+r", "ctrl+p", "f2", "f4", "alt+g":
			return false, nil
		}
	}
	if m.multiComposer() {
		m.pasteSelection = nil
		m.pathHints = nil
	}
	handled, cmd := m.input.HandleKey(k)
	m.rejectPartialChipEdit(before)
	if handled || hadSelection || before != m.input.Value() {
		m.pasteSelection = nil
		m.resize()
	}
	if handled && (k.String() == "home" || k.String() == "end") {
		m.snapChipCursor()
	}
	return handled, cmd
}
func (m *model) composerRows() []composerRow           { return m.input.Rows() }
func composerRowIndex(rows []composerRow, pos int) int { return bed.RowIndex(rows, pos) }
func composerRowEnd(rows []composerRow, i int) int     { return bed.RowEnd(rows, i) }
func (m *model) composerScroll(rows []composerRow) int { return m.input.ScrollOffset(rows) }
func (m *model) composerPoint(x, y int) int            { return m.input.Point(x, y) }
func (m *model) composerMouse(v tea.MouseMsg) bool {
	if !m.composerAvailable() {
		return false
	}
	m.prepareEditor()
	top := m.height - m.input.Height() - 2 - m.terminalHeight()
	v.X -= m.contentOffset() + 3
	v.Y -= top
	if !m.input.HandleMouse(v) {
		return false
	}
	if v.Button != tea.MouseButtonWheelUp && v.Button != tea.MouseButtonWheelDown {
		m.panelFocus, m.focusList, m.focusApproval = false, false, false
		m.textSelection = nil
		m.pasteSelection = nil
	}
	m.resize()
	return true
}
func (m *model) composerSelectionView(view string) string {
	if !m.composerAvailable() {
		return view
	}
	m.prepareEditor()
	return m.input.RenderSelection(view)
}
func (m *model) scrollComposer(down bool) bool {
	if !m.input.Scroll(down) {
		return false
	}
	m.resize()
	return true
}
func (m *model) composerScrollbar(view string) string {
	m.input.ScrollbarStyle = muted
	m.input.ScrollbarTrackStyle = zeroStyle
	return m.input.RenderScrollbar(view, m.width-2)
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

// focusComposer retains the command for Update to dispatch even when a view
// handler has no command return value. Repeated focus calls replace older timers.
func (m *model) focusComposer() tea.Cmd {
	if !m.composerBlink {
		return m.input.Focus()
	}
	m.input.Cursor.SetMode(cursor.CursorBlink)
	m.composerFocusCmd = m.input.Focus()
	return nil
}

func (m *model) composerIndentKey(k tea.KeyMsg) (bool, tea.Cmd) {
	if k.Paste || (k.String() != "tab" && k.String() != "shift+tab") || !m.composerAvailable() || m.panelFocus || m.focusList || m.focusApproval || m.approvalHeight() > 0 || m.errorVisible() {
		return false, nil
	}
	m.prepareEditor()
	before := m.input.Value()
	handled, cmd := m.input.HandleKey(k)
	m.rejectPartialChipEdit(before)
	if handled {
		m.pasteSelection = nil
		m.resize()
	}
	return handled, cmd
}

func (m *model) rejectPartialChipEdit(before string) {
	if !partialPasteEdit(before, m.input.Value(), m.pastes) {
		return
	}
	m.input.Undo()
	if m.input.Value() != before {
		m.input.SetValue(before)
	}
	m.notice = "Paste chips are indivisible; Ctrl+P to preview or delete."
}

// Product completions are single-target. Keep them hidden while bed owns a
// selection set; they can be used again after Escape retains the primary.
func (m *model) multiComposer() bool { return len(m.input.Selections()) > 1 }
