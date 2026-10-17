package tui

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
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
	return m.workflow == nil && !m.questionFocused() && m.report == nil && m.modelPicker == nil && m.restartConfirm == nil && m.pasteDialog == nil && m.redactDialog == nil && !m.panelFocus
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
	case "ctrl+d":
		return tea.Quit
	case "esc", "x", "tab":
		m.toolSelector = nil
		m.focusComposer()
		return nil
	}
	targets := m.toolTargets()
	if len(targets) == 0 {
		m.toolSelector = nil
		m.focusComposer()
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
		_, cmd := m.openPreviewSequence(targets[index].seq)
		return cmd
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
			rows[row] = focus.Bold(true).Render(clip("› "+text, m.view.Width))
		}
		break
	}
}

// previewSubject answers whether a journal coordinate is inspectable at all,
// from what is loaded. It has to be synchronous: a click on a row that is not a
// tool must fall through to text selection in the same frame, which cannot wait
// for a round trip.
func (m *model) previewSubject(seq uint64) *api.Event {
	s := m.current()
	if s == nil {
		return nil
	}
	for _, e := range m.events[s.Id] {
		if e.Seq != seq {
			continue
		}
		if e.Kind == "approval" || e.Kind == "tool_call" || e.Kind == "tool_result" {
			return e
		}
		return nil
	}
	return nil
}

// previewFromRecords builds the detail panel from the records EventDetails
// returned. They are the subject's own -- the server answers with everything
// sharing its request identity, including the approval it waited on -- so there
// is no searching here for which records belong together.
//
// Reading them from the server rather than from the loaded list is what lets
// the display list stop being a list of records. A row carries a tool's name,
// shape and state; the bytes it ran and produced were never on it. See cxz#155.
func previewFromRecords(agent string, selected *api.Event, events []*api.Event) *filePreview {
	var call, result *api.Event
	if selected.Kind == "approval" {
		approval := agentview.ApprovalView(agent, selected.Text, selected.Payload)
		state := "No resolution recorded for this request."
		for _, e := range events {
			if selected.RequestId != "" && e.Kind == "approval_resolved" && e.RequestId == selected.RequestId && e.RunId == selected.RunId && e.Seq > selected.Seq {
				state = e.Text
			}
		}
		return &filePreview{title: "Approval · " + approval.Title, source: "Approval status: " + state + "\nThis is an approval record, not a tool execution result.\n\n" + approval.Detail, language: "text"}
	}
	if selected.Kind != "tool_call" && selected.Kind != "tool_result" {
		return nil
	}
	if selected.Kind == "tool_call" {
		call = selected
	} else {
		result = selected
	}
	if selected.RequestId != "" {
		for _, e := range events {
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
	input := &filePreview{source: "Input was not retained for this call.", language: "text"}
	if call != nil {
		input = toolInputPreview(agent, call)
	}
	output := toolOutputPreview(agent, call, result)
	title := input.title
	if output.title != "" {
		title = output.title
	}
	if call != nil {
		if submitted := previewFor(agent, call); submitted != nil {
			title = submitted.title
		}
	}
	if title == "" {
		title = "Tool details"
	}
	p := &filePreview{title: title, tabs: &[2]previewTab{
		{source: input.source, language: input.language},
		{source: output.source, language: output.language},
	}}
	// Preserve the useful default: submitted edits, otherwise the recorded output.
	tab := 1
	if result == nil || (call != nil && agent == "claude" && (call.Text == "Write" || call.Text == "Edit")) {
		tab = 0
	}
	p.selectTab(tab)
	return p
}

func rawToolPreview(e *api.Event) *filePreview {
	var value any
	source := string(e.Payload)
	if json.Unmarshal(e.Payload, &value) == nil {
		b, _ := json.MarshalIndent(value, "", "  ")
		source = string(b)
	}
	if source == "" {
		source = e.Text
	}
	return &filePreview{title: e.Text, source: source, language: "json"}
}

func toolInputPreview(provider string, call *api.Event) *filePreview {
	if p := previewFor(provider, call); p != nil {
		return p
	}
	root := fields(call.Payload)
	if provider == "claude" && call.Text == "Bash" {
		return &filePreview{title: "Bash", source: root.text("command"), language: "bash"}
	}
	if provider == "codex" && root.object("item").text("type") == "commandExecution" {
		return &filePreview{title: "Bash", source: root.object("item").text("command"), language: "bash"}
	}
	return rawToolPreview(call)
}

func toolOutputPreview(provider string, call, result *api.Event) *filePreview {
	if result != nil {
		if p := previewFor(provider, result); p != nil {
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
			name = "Bash"
		}
		if source != "" {
			return &filePreview{title: name, source: source, language: language}
		}
		return rawToolPreview(result)
	}
	return &filePreview{source: "No output available yet.", language: "text"}
}

// toolPreviewLoaded carries a tool's own records back to the panel that asked
// for them. It is matched by session and sequence, so a reader who moved on
// before the answer arrived is not shown the wrong tool.
type toolPreviewLoaded struct {
	session, agent string
	seq            uint64
	subject        *api.Event
	events         []*api.Event
	err            error
}

func (m *model) loadToolPreview(id, agent string, seq uint64, subject *api.Event) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		batch, err := client.EventDetails(ctx, &api.EventDetailsRequest{SessionId: id, Seq: seq})
		out := toolPreviewLoaded{session: id, agent: agent, seq: seq, subject: subject, err: err}
		if batch != nil {
			out.events = batch.Events
		}
		return out
	}
}

func (m *model) acceptToolPreview(v toolPreviewLoaded) {
	p := m.filePreview
	if p == nil || p.session != v.session || p.seq != v.seq {
		return
	}
	if v.err != nil {
		p.tabs, p.source, p.language = nil, "Tool details are unavailable: "+v.err.Error(), "text"
		p.rendered = ""
		m.resize()
		m.render()
		return
	}
	// The subject itself may be the newest record of the three, so keep it in
	// the set rather than trusting the reply to contain it.
	subject := v.subject
	for _, e := range v.events {
		if e.Seq == v.seq {
			subject = e
		}
	}
	next := previewFromRecords(v.agent, subject, v.events)
	if next == nil {
		return
	}
	next.session, next.seq, next.focused, next.width = p.session, p.seq, p.focused, 0
	m.filePreview = next
	m.resize()
	m.render()
}

func (m *model) openPreviewSequence(seq uint64) (bool, tea.Cmd) {
	s := m.current()
	if s == nil {
		return false, nil
	}
	subject := m.previewSubject(seq)
	if subject == nil {
		return false, nil
	}
	// Opened now and filled when the records arrive. The panel has to open in
	// this frame: whether the click was consumed is answered here, and a reader
	// who waited for a round trip to find out would see the click do nothing.
	p := &filePreview{title: "Tool details", source: "Loading…", language: "text", seq: seq}
	p.session = s.Id
	p.focused = true
	m.filePreview = p
	m.textSelection = nil
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
	return true, m.loadToolPreview(s.Id, s.Agent, seq, subject)
}
func (m *model) closeFilePreview() {
	m.filePreview = nil
	m.resize()
	m.render()
	if !m.selectingTools() {
		m.focusComposer()
	}
}
func (m *model) filePreviewKey(k tea.KeyMsg) tea.Cmd {
	if k.Paste {
		return nil
	}
	p := m.filePreview
	page := max(1, m.previewHeight()-previewFrameRows)
	if m.previewSideWidth() > 0 {
		page = max(1, m.height-previewFrameRows)
	}
	switch k.String() {
	case "ctrl+c":
		m.copyText(p.source)
	case "ctrl+d":
		return tea.Quit
	case "x", "esc":
		m.closeFilePreview()
	case "tab":
		p.focused = false
		if !m.selectingTools() {
			m.focusComposer()
		}
	case "left":
		if p.tabs != nil {
			m.textSelection = nil
			p.selectTab(0)
		}
	case "right":
		if p.tabs != nil {
			m.textSelection = nil
			p.selectTab(1)
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
