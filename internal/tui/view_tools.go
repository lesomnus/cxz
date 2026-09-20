package tui

import (
	"encoding/json"
	"math"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

type toolSelector struct {
	session string
	seq     uint64
}
type toolTarget struct {
	seq uint64
	row int
}

func (m *model) selectingTools() bool {
	s := m.current()
	return m.toolSelector != nil && s != nil && m.toolSelector.session == s.Id && !m.projectView && !m.accountView
}
func (m *model) previewInteraction() bool {
	return m.workflow == nil && m.questionDialog == nil && m.report == nil && m.modelPicker == nil && m.restartConfirm == nil && m.pasteDialog == nil && m.redactDialog == nil && !m.panelFocus
}

// Use rendered journal coordinates: hidden paired results are not separate targets.
func (m *model) toolTargets() []toolTarget {
	s := m.current()
	if s == nil {
		return nil
	}
	eligible := map[uint64]bool{}
	for _, e := range m.events[s.Id] {
		if e.Kind == "tool_call" || e.Kind == "tool_result" {
			eligible[e.Seq] = true
		}
	}
	var targets []toolTarget
	for row, pos := range m.historyPositions {
		seq := uint64(math.Floor(pos))
		if !eligible[seq] || pos != float64(seq) {
			continue
		}
		if len(targets) > 0 && targets[len(targets)-1].seq == seq {
			targets[len(targets)-1].row = row
		} else {
			targets = append(targets, toolTarget{seq, row})
		}
	}
	return targets
}
func (m *model) viewCommand(text string) tea.Cmd {
	if text != "/view" {
		m.openReport("/view", "Usage: /view\n↑/↓ select a tool row · Enter open · Esc return to input")
		return nil
	}
	targets := m.toolTargets()
	if len(targets) == 0 {
		m.notice = "No inspectable tool rows loaded; scroll up to load earlier history."
		return nil
	}
	selected := targets[len(targets)-1]
	for _, t := range targets {
		if t.row >= m.view.YOffset && t.row < m.view.YOffset+m.view.Height {
			selected = t
		}
	}
	m.toolSelector = &toolSelector{m.current().Id, selected.seq}
	if m.filePreview != nil {
		m.filePreview.focused = false
	}
	m.focusApproval = false
	m.focusList = false
	m.input.Blur()
	m.revealTool(selected.row)
	return nil
}
func (m *model) revealTool(row int) {
	// Leave space for the pinned prompt at the top of a scrolled transcript.
	if row < m.view.YOffset+2 {
		m.view.SetYOffset(max(0, row-2))
	}
	if row >= m.view.YOffset+m.view.Height {
		m.view.SetYOffset(max(0, row-m.view.Height+1))
	}
}
func (m *model) toolSelectorKey(k tea.KeyMsg) tea.Cmd {
	if k.Paste {
		return nil
	}
	switch k.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc", "x", "tab":
		m.toolSelector = nil
		m.input.Focus()
		return nil
	}
	targets := m.toolTargets()
	if len(targets) == 0 {
		m.toolSelector = nil
		m.input.Focus()
		return nil
	}
	index := 0
	for i, t := range targets {
		if t.seq == m.toolSelector.seq {
			index = i
			break
		}
	}
	switch k.String() {
	case "up":
		if index == 0 {
			m.view.GotoTop()
			return m.loadOlderHistory()
		}
		index--
	case "down":
		index = min(len(targets)-1, index+1)
	case "home", "ctrl+home":
		index = 0
	case "end", "ctrl+end":
		index = len(targets) - 1
	case "enter":
		m.openPreviewSequence(targets[index].seq)
		return nil
	default:
		return nil
	}
	m.toolSelector.seq = targets[index].seq
	m.revealTool(targets[index].row)
	return nil
}
func (m *model) toolSelectorView(rows []string) {
	if !m.selectingTools() {
		return
	}
	for _, t := range m.toolTargets() {
		if t.seq != m.toolSelector.seq {
			continue
		}
		row := t.row - m.view.YOffset
		if row >= 0 && row < len(rows) {
			text := strings.TrimPrefix(ansi.Strip(rows[row]), "  ")
			rows[row] = accent.Bold(true).Render(clip("› "+text, m.width))
		}
		break
	}
}

func (m *model) previewForSequence(seq uint64) *filePreview {
	s := m.current()
	if s == nil {
		return nil
	}
	var selected, call, result *api.Event
	for _, e := range m.events[s.Id] {
		if e.Seq == seq {
			selected = e
			break
		}
	}
	if selected == nil || (selected.Kind != "tool_call" && selected.Kind != "tool_result") {
		return nil
	}
	if selected.Kind == "tool_call" {
		call = selected
	} else {
		result = selected
	}
	if selected.RequestId != "" {
		for _, e := range m.events[s.Id] {
			if e.RequestId != selected.RequestId || e.RunId != selected.RunId {
				continue
			}
			if e.Kind == "tool_call" {
				call = e
			}
			if e.Kind == "tool_result" {
				result = e
			}
		}
	}
	// Write/Edit show the submitted content/diff, even if the result is just "success".
	if call != nil {
		if p := previewFor(s.Agent, call); p != nil && s.Agent != "codex" {
			return p
		}
	}
	if result != nil {
		if p := previewFor(s.Agent, result); p != nil {
			return p
		}
		root := fields(result.Payload)
		source := root.object("item").text("aggregatedOutput")
		if source == "" {
			source = root.text("content")
		}
		if source == "" {
			var blocks []struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(root["content"], &blocks) == nil {
				for _, b := range blocks {
					source += b.Text + "\n"
				}
			}
		}
		if source == "" && len(result.Payload) == 0 {
			source = result.Text
		}
		name, language := "Tool result", "text"
		if call != nil {
			if call.Text != "" {
				name = call.Text
			}
			if name == "Read" {
				language = fields(call.Payload).text("file_path")
				name += " · " + language
			}
		}
		if root.object("item").text("type") == "commandExecution" {
			name = "Bash · output"
		}
		if source != "" {
			return &filePreview{title: name, source: source, language: language}
		}
		selected = result
	}
	if p := previewFor(s.Agent, selected); p != nil {
		return p
	}
	if call != nil && call.Text == "Bash" && result == nil {
		return &filePreview{title: "Bash · script", source: fields(call.Payload).text("command"), language: "bash"}
	}
	var value any
	source := string(selected.Payload)
	if json.Unmarshal(selected.Payload, &value) == nil {
		b, _ := json.MarshalIndent(value, "", "  ")
		source = string(b)
	}
	if source == "" {
		source = selected.Text
	}
	return &filePreview{title: selected.Text + " · " + strings.ReplaceAll(selected.Kind, "_", " "), source: source, language: "json"}
}
func (m *model) openPreviewSequence(seq uint64) bool {
	p := m.previewForSequence(seq)
	if p == nil {
		return false
	}
	p.session = m.current().Id
	p.focused = true
	m.filePreview = p
	m.input.Blur()
	m.focusApproval = false
	m.resize()
	m.render()
	if m.selectingTools() {
		for _, t := range m.toolTargets() {
			if t.seq == seq {
				m.toolSelector.seq = seq
				m.revealTool(t.row)
				break
			}
		}
	}
	return true
}
func (m *model) closeFilePreview() {
	m.filePreview = nil
	m.resize()
	m.render()
	if !m.selectingTools() {
		m.input.Focus()
	}
}
func (m *model) filePreviewKey(k tea.KeyMsg) tea.Cmd {
	if k.Paste {
		return nil
	}
	p := m.filePreview
	page := max(1, m.previewHeight()-2)
	if m.previewSideWidth() > 0 {
		page = max(1, m.height-2)
	}
	switch k.String() {
	case "ctrl+c":
		return tea.Quit
	case "x", "esc":
		m.closeFilePreview()
	case "tab":
		p.focused = false
		if !m.selectingTools() {
			m.input.Focus()
		}
	case "up":
		p.offset = max(0, p.offset-1)
	case "down":
		p.offset++
	case "pgup":
		p.offset = max(0, p.offset-page)
	case "pgdown":
		p.offset += page
	case "home", "ctrl+home":
		p.offset = 0
	case "end", "ctrl+end":
		p.offset = int(^uint(0) >> 1)
	}
	return nil
}
