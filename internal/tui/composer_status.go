package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func (m *model) navigationNotice() string {
	if m.errorDialog != nil && m.notice == m.errorDialog.text {
		return ""
	}
	return m.notice
}

// Return the displayed row and the original notice's column before clipping.
// Scroll/selection/interrupt hints replace the notice, so they have no copy target.
func (m *model) composerStatus() (text, message string, column int) {
	message = m.navigationNotice()
	text = warning.Render(pickerLabel(message))
	if !m.view.AtBottom() {
		text = muted.Render(m.scrollStatus())
		message = ""
	} else if s := m.current(); s != nil && s.State != "working" && s.State != "idle" && s.State != "waiting_input" {
		prefix := warning.Render(pickerLabel(s.State)) + " · "
		text = prefix + text
		column += ansi.StringWidth(prefix)
	}
	if m.selectingTools() && !(m.previewVisible() && m.filePreview.focused) {
		text = accent.Render("/view · ↑/↓ select · Enter open · Esc input")
		message = ""
	}
	if background := m.backgroundStatus(); background != "" {
		text = background + " · " + text
		column += ansi.StringWidth(background + " · ")
	}
	if s := m.current(); s != nil && s.State == "waiting_input" && m.interruptKey == s.Id+"/"+s.RunId && time.Now().Before(m.interruptUntil) {
		text = warning.Render("Esc again to interrupt (3s)")
		message = ""
	}
	return
}

func (m *model) composerStatusMouse(v tea.MouseMsg) bool {
	if (m.projectView && !m.creating) || m.accountView || m.workflow != nil || m.settingsPage != nil || m.memoryPage != nil || m.width < 40 || m.height < 14 {
		return false
	}
	if v.Y != m.height-m.input.Height()-m.terminalHeight()-4 {
		return false
	}
	_, message, start := m.composerStatus()
	if message == "" {
		return false
	}
	start += 2 // Same inset as sessionScreen's status row.
	right := m.width
	if label := m.recordingLabel(time.Now()); label != "" {
		right -= ansi.StringWidth(label) + 1
	}
	end := min(right, start+ansi.StringWidth(pickerLabel(message)))
	if v.X < start || v.X >= end {
		return false
	}
	if v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress {
		m.rememberNotice()
		m.copyText(message)
	}
	return true
}
