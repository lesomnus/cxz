package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const errorContentRows = 3
const errorDialogRows = errorContentRows + 3 // Header, scroll hint, focus rule.

type errorDialog struct {
	text, hover string
	offset      int
	focused     bool
	width       int
	rows        []string
}

// Keep failures separate from transient command receipts and clipboard feedback.
// Opening a failure must not take focus away from a draft being typed.
func (m *model) showError(text string) {
	m.notice = text
	m.rememberNotice()
	if m.errorDialog != nil && m.errorDialog.text == text {
		return
	}
	focused := m.errorFocused()
	m.errorDialog = &errorDialog{text: text, focused: focused}
	m.resize()
	m.render()
}

func (m *model) errorVisible() bool {
	// On minimal screens the question already carries its submission error.
	// Keep the full error for later rather than leave an invisible modal owning
	// the keyboard. Both panels remain visible when there is room for them.
	if m.questionDialog != nil && m.height-m.input.Height()-5 < errorDialogRows+questionChromeRows+2 {
		return false
	}
	return m.errorDialog != nil && m.width >= 40 && m.height >= 14 &&
		m.settingsPage == nil && m.memoryPage == nil && m.workflow == nil
}

func (m *model) errorFocused() bool {
	return m.errorVisible() && m.errorDialog.focused && m.errorInteraction()
}

func (m *model) errorInteraction() bool {
	return m.questionDialog == nil && m.report == nil && m.modelPicker == nil &&
		m.restartConfirm == nil && m.pasteDialog == nil && m.redactDialog == nil
}

func (m *model) errorHeight() int {
	if m.errorVisible() {
		return errorDialogRows
	}
	return 0
}

// Coordinates are shared by rendering and hit testing, including pages without
// a composer. They are global terminal coordinates, like tool preview buttons.
func (m *model) errorBounds() (x, y, width int) {
	x, width = m.contentOffset()+2, m.width-4
	y = m.height - m.input.Height() - m.terminalHeight() - 4 - errorDialogRows
	if m.accountView || m.projectView && !m.creating {
		y = m.height - errorDialogRows
	}
	if !m.panelVisible() && (m.panelFocus || m.projectView) && !m.accountView && !m.creating {
		x, y, width = 2, m.height-errorDialogRows, m.panelScreenWidth()-4
	}
	return
}

func (m *model) errorLines(width int) []string {
	d := m.errorDialog
	if d.width != width || d.rows == nil {
		text := strings.ReplaceAll(safeText(d.text), "\t", "    ")
		d.rows = strings.Split(ansi.Hardwrap(text, max(1, width-2), true), "\n")
		d.width = width
	}
	d.offset = max(0, min(d.offset, max(0, len(d.rows)-errorContentRows)))
	return d.rows
}

func (m *model) errorRows(width int) string {
	d := m.errorDialog
	rows := m.errorLines(width)
	inner := max(1, width-2)
	body := []string{failure.Render("Error")}
	for i := 0; i < errorContentRows; i++ {
		line := ""
		if d.offset+i < len(rows) {
			line = rows[d.offset+i]
		}
		body = append(body, answer.Render(line))
	}
	hint := "Tab/click to focus"
	if m.errorFocused() {
		hint = "↑↓ scroll · x close · Tab back"
	}
	body = append(body, muted.Render(fmt.Sprintf("%d–%d/%d · %s", d.offset+1, min(len(rows), d.offset+errorContentRows), len(rows), hint)), "")
	for i, line := range body {
		line = clip(line, inner)
		body[i] = panelBackground(" " + line + strings.Repeat(" ", max(0, inner-ansi.StringWidth(line))) + " ")
	}
	body[0] = overlayButton(body[0], width-8, " ⧉ ", d.hover == "copy", 236)
	body[0] = overlayButton(body[0], width-4, "[×]", d.hover == "close", 236)
	if m.errorFocused() {
		body[len(body)-1] = panelBackground(accent.Render(strings.Repeat("─", width)))
	}
	return strings.Join(body, "\n")
}

func (m *model) errorOverlay(view string) string {
	if !m.errorVisible() {
		return view
	}
	x, y, width := m.errorBounds()
	// View's main column has not yet been joined to the project sidebar.
	x -= m.contentOffset()
	lines := strings.Split(view, "\n")
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	for i, row := range strings.Split(m.errorRows(width), "\n") {
		lines[y+i] = strings.Repeat(" ", max(0, x)) + row
	}
	return strings.Join(lines, "\n")
}

func (m *model) focusError() {
	m.errorDialog.focused = true
	m.panelFocus, m.focusApproval, m.focusList = false, false, false
	m.toolSelector, m.textSelection, m.pathHints = nil, nil, nil
	if m.filePreview != nil {
		m.filePreview.focused = false
	}
	if p := m.terminal(); p != nil {
		p.focused = false
	}
	m.input.Blur()
}

func (m *model) leaveError() tea.Cmd {
	m.errorDialog.focused = false
	if m.projectView && !m.accountView && !m.creating {
		m.focusPanel()
		return nil
	}
	return m.input.Focus()
}

func (m *model) closeError() tea.Cmd {
	var cmd tea.Cmd
	if m.errorDialog.focused {
		cmd = m.leaveError()
	}
	if m.notice == m.errorDialog.text {
		m.notice = ""
	}
	m.errorDialog = nil
	m.resize()
	m.render()
	return cmd
}

func (m *model) errorKey(k tea.KeyMsg) tea.Cmd {
	if k.Paste {
		return nil
	}
	d := m.errorDialog
	switch k.String() {
	case "up":
		d.offset--
	case "down":
		d.offset++
	case "pgup":
		d.offset -= errorContentRows
	case "pgdown":
		d.offset += errorContentRows
	case "home":
		d.offset = 0
	case "end":
		_, _, width := m.errorBounds()
		d.offset = len(m.errorLines(width))
	case "esc", "x":
		return m.closeError()
	case "tab", "shift+tab":
		return m.leaveError()
	case "ctrl+q":
		d.focused = false
		m.focusPanel()
	}
	_, _, width := m.errorBounds()
	m.errorLines(width)
	return nil
}

func (m *model) errorMouse(v tea.MouseMsg) (bool, tea.Cmd) {
	if m.errorDialog == nil {
		return false, nil
	}
	d := m.errorDialog
	d.hover = ""
	if !m.errorVisible() || !m.errorInteraction() {
		return false, nil
	}
	x, y, width := m.errorBounds()
	if v.X < x || v.X >= x+width || v.Y < y || v.Y >= y+errorDialogRows {
		if d.focused && v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress {
			return false, m.leaveError()
		}
		return false, nil
	}
	if v.Y == y {
		if v.X >= x+width-8 && v.X < x+width-5 {
			d.hover = "copy"
		} else if v.X >= x+width-4 && v.X < x+width-1 {
			d.hover = "close"
		}
	}
	switch v.Button {
	case tea.MouseButtonWheelUp:
		d.offset -= errorContentRows
	case tea.MouseButtonWheelDown:
		d.offset += errorContentRows
	case tea.MouseButtonLeft:
		if v.Action == tea.MouseActionPress {
			switch d.hover {
			case "copy":
				m.copyText(d.text)
			case "close":
				return true, m.closeError()
			default:
				m.focusError()
			}
		}
	}
	m.errorLines(width)
	return true, nil
}
