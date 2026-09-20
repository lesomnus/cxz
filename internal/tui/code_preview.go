package tui

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/quick"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func highlightCode(source, language string) string {
	source = safeText(source)
	if lipgloss.ColorProfile().Name() == "Ascii" {
		return source
	}
	if lexer := lexers.Match(language); lexer != nil {
		language = lexer.Config().Name
	}
	var out strings.Builder
	if err := quick.Highlight(&out, source, language, "terminal16m", "dracula"); err != nil {
		return source
	}
	return strings.TrimSuffix(out.String(), "\n")
}

// Keep a bounded tail even when a process emits a single enormous line.
func outputTail(text string) string {
	if len(text) > 32768 {
		text = text[len(text)-32768:]
	}
	rows := strings.Split(text, "\n")
	if len(rows) > 7 {
		rows = rows[len(rows)-7:]
	}
	return strings.Join(rows, "\n")
}
func liveOutputView(text string, width int) string {
	rows := strings.Split(ansi.Hardwrap(safeText(strings.TrimRight(text, "\n")), max(1, width-6), true), "\n")
	rows = rows[max(0, len(rows)-6):]
	for i := range rows {
		rows[i] = "    " + muted.Render("│ "+rows[i])
	}
	return strings.Join(rows, "\n")
}

type filePreview struct {
	session, title, source, language string
	offset                           int
	focused                          bool
	rendered                         string
	width                            int
}

// Preview the original tool payload, never the current workspace file.
func previewFor(provider string, e *api.Event) *filePreview {
	p := fields(e.Payload)
	if provider == "claude" {
		path := p.text("file_path")
		switch e.Text {
		case "Write":
			return &filePreview{title: "Write · " + path, source: p.text("content"), language: path}
		case "Edit":
			var rows []string
			for _, part := range []struct{ key, prefix string }{{"old_string", "-"}, {"new_string", "+"}} {
				value := p.text(part.key)
				if value != "" {
					for _, row := range strings.Split(strings.TrimSuffix(value, "\n"), "\n") {
						rows = append(rows, part.prefix+row)
					}
				}
			}
			return &filePreview{title: "Edit · " + path, source: strings.Join(rows, "\n"), language: "diff"}
		}
	}
	if provider == "codex" && p.object("item").text("type") == "fileChange" {
		var changes []struct{ Path, Diff string }
		if json.Unmarshal(p.object("item")["changes"], &changes) != nil {
			return nil
		}
		var rows []string
		for _, c := range changes {
			rows = append(rows, c.Path+"\n"+c.Diff)
		}
		return &filePreview{title: "File changes", source: strings.Join(rows, "\n\n"), language: "diff"}
	}
	return nil
}
func (m *model) previewVisible() bool {
	s := m.current()
	return m.filePreview != nil && s != nil && m.filePreview.session == s.Id && !m.projectView && !m.accountView && m.workflow == nil
}
func (m *model) previewSideWidth() int {
	if !m.previewVisible() {
		return 0
	}
	remaining := m.terminalWidth - m.contentOffset() - m.width - 2
	if remaining < 40 {
		return 0
	}
	return min(80, remaining)
}
func (m *model) previewHeight() int {
	if !m.previewVisible() || m.previewSideWidth() > 0 {
		return 0
	}
	return min(8, max(0, m.height-m.input.Height()-5-m.approvalHeight()-m.terminalHeight()-4))
}
func (m *model) previewRows(width, height int) string {
	p := m.filePreview
	if p.width != width || p.rendered == "" {
		p.rendered = ansi.Hardwrap(highlightCode(p.source, p.language), max(1, width-4), true)
		p.width = width
	}
	rows := strings.Split(p.rendered, "\n")
	count := max(0, height-2)
	p.offset = max(0, min(p.offset, max(0, len(rows)-count)))
	border := teal
	focus := ""
	if p.focused && m.previewInteraction() {
		border = accent
		focus = "● "
	}
	title := clip(focus+safeText(p.title), max(1, width-7))
	header := border.Render("╭" + title + strings.Repeat("─", max(0, width-5-ansi.StringWidth(title))) + "[×]╮")
	body := []string{header}
	inner := max(1, width-4)
	for i := 0; i < count; i++ {
		line := ""
		if p.offset+i < len(rows) {
			line = rows[p.offset+i]
		}
		line = clip(line, inner)
		body = append(body, border.Render("│")+" "+line+strings.Repeat(" ", max(0, inner-ansi.StringWidth(line)))+" "+border.Render("│"))
	}
	hint := "click to focus"
	if p.focused && m.previewInteraction() {
		hint = "↑↓ scroll · x close · Tab back"
	}
	footer := clip(fmt.Sprintf("%d–%d/%d · %s", p.offset+1, min(len(rows), p.offset+count), len(rows), hint), max(1, width-2))
	body = append(body, border.Render("╰"+footer+strings.Repeat("─", max(0, width-2-ansi.StringWidth(footer)))+"╯"))
	return strings.Join(body, "\n")
}
func (m *model) openFilePreview(y int) bool {
	index := m.view.YOffset + y
	if index < 0 || index >= len(m.historyPositions) {
		return false
	}
	return m.openPreviewSequence(uint64(math.Floor(m.historyPositions[index])))
}

func (m *model) filePreviewMouse(v tea.MouseMsg) bool {
	if !m.previewVisible() || m.questionDialog != nil || m.report != nil || m.modelPicker != nil || m.restartConfirm != nil || m.pasteDialog != nil || m.redactDialog != nil {
		return false
	}
	width, height := m.width, m.previewHeight()
	x, y := m.contentOffset(), m.view.Height+2+m.approvalHeight()
	if side := m.previewSideWidth(); side > 0 {
		x = m.contentOffset() + m.width + 2
		y = 0
		width = side
		height = m.height
	}
	if height < 2 || v.X < x || v.X >= x+width || v.Y < y || v.Y >= y+height {
		if v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress {
			m.filePreview.focused = false
			m.toolSelector = nil
			m.input.Focus()
		}
		return false
	}
	if v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress {
		m.filePreview.focused = true
		m.input.Blur()
	}
	switch v.Button {
	case tea.MouseButtonWheelUp:
		m.filePreview.offset = max(0, m.filePreview.offset-3)
	case tea.MouseButtonWheelDown:
		m.filePreview.offset += 3
	case tea.MouseButtonLeft:
		if v.Action == tea.MouseActionPress && v.Y == y && v.X >= x+width-3 {
			m.closeFilePreview()
		}
	}
	return true
}
