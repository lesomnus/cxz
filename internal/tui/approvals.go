package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

type approvalResult struct {
	id, run, request string
	automatic        bool
	err              error
}

func question(p *api.Event) bool {
	return p.Text == "AskUserQuestion" || p.Text == "item/tool/requestUserInput"
}
func automaticApproval(p *api.Event) bool {
	switch p.Text {
	case "Bash", "Read", "Edit", "Write", "Glob", "Grep", "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval":
		return true
	}
	return false // Unknown methods and questions remain manual.
}
func permissionState(state string) bool {
	return state == "idle" || state == "working" || state == "waiting_input"
}
func (m *model) selectedApproval() *api.Event {
	s := m.current()
	if s == nil || len(s.Pending) == 0 {
		return nil
	}
	for _, p := range s.Pending {
		if p.RequestId == m.approvalID {
			return p
		}
	}
	return s.Pending[0]
}
func (m *model) replyApproval(p *api.Event, allow bool, answers string, automatic bool) tea.Cmd {
	s := m.current()
	if s == nil || p == nil {
		m.notice = "No pending approval"
		return nil
	}
	if allow && question(p) && answers == "" {
		m.notice = "This request needs an answer. Use /approval to inspect, then /answer {\"question\":\"answer\"}."
		return nil
	}
	if answers != "" {
		var values map[string]string
		if json.Unmarshal([]byte(answers), &values) != nil {
			m.notice = "/answer requires a JSON string map"
			return nil
		}
	}
	key := s.Id + "/" + s.RunId + "/" + p.RequestId
	if m.approvalSent == nil {
		m.approvalSent = map[string]bool{}
	}
	if m.approvalSent[key] {
		m.notice = "Decision already sent; waiting for updated approval state"
		return nil
	}
	m.approvalSent[key] = true
	id, run, request := s.Id, s.RunId, p.RequestId
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.client.Reply(ctx, &api.Answer{SessionId: id, RunId: run, RequestId: request, ClientId: core.ID(), Allow: allow, AnswersJson: answers})
		return approvalResult{id: id, run: run, request: request, automatic: automatic, err: err}
	}
}
func (m *model) autoApprove() tea.Cmd {
	s := m.current()
	if s == nil || s.RunId == "" || !permissionState(s.State) || m.fullPermission[s.Id] != s.RunId || m.projectView || m.accountView {
		return nil
	}
	for _, p := range s.Pending {
		if automaticApproval(p) && !m.approvalSent[s.Id+"/"+s.RunId+"/"+p.RequestId] {
			return m.replyApproval(p, true, "", true)
		}
	}
	return nil
}
func (m *model) permissionCommand(text string) tea.Cmd {
	s := m.current()
	if s == nil {
		m.notice = "Select a session first"
		return nil
	}
	parts := strings.Fields(text)
	if len(parts) != 2 || (parts[1] != "full" && parts[1] != "ask") {
		m.recordLocal("/permission", "Usage: /permission full | ask\nFull automatically approves known tool/command/file/permission requests for this session run while this TUI is connected. Questions still require answers.")
		return nil
	}
	if m.fullPermission == nil {
		m.fullPermission = map[string]string{}
	}
	if parts[1] == "ask" {
		delete(m.fullPermission, s.Id)
		m.recordLocal("/permission", "Manual approval enabled. Already dispatched decisions cannot be retracted.")
		return nil
	}
	if s.RunId == "" || !permissionState(s.State) {
		m.recordLocal("/permission", "Session must be idle, working or waiting_input before enabling full permission.")
		return nil
	}
	m.fullPermission[s.Id] = s.RunId
	m.recordLocal("/permission", "FULL permission enabled for this session run while attached. Pending and future known tool/command/file/permission requests may execute without confirmation. Questions and unknown requests stay manual. /permission ask disables it. Disconnect/restart resets it.")
	return m.autoApprove()
}
func (m *model) recordLocal(command, text string) {
	id := ""
	if s := m.current(); s != nil {
		id = s.Id
	}
	if m.localHelp == nil {
		m.localHelp = map[string]uint64{}
	}
	if m.localOutput == nil {
		m.localOutput = map[string]string{}
	}
	if m.localReports == nil {
		m.localReports = map[string]string{}
	}
	m.localHelp[id], m.localOutput[id], m.localReports[id] = m.cursor[id], command, text
	m.resize()
	m.render()
	m.view.GotoBottom()
}
func (m *model) approvalKey(k tea.KeyMsg) tea.Cmd {
	p := m.selectedApproval()
	if p == nil {
		m.focusApproval = false
		return m.input.Focus()
	}
	s := m.current()
	switch k.String() {
	case "esc":
		m.focusApproval = false
		return m.input.Focus()
	case "up", "down":
		for i, v := range s.Pending {
			if v.RequestId == p.RequestId {
				delta := 1
				if k.String() == "up" {
					delta = len(s.Pending) - 1
				}
				m.approvalID = s.Pending[(i+delta)%len(s.Pending)].RequestId
				break
			}
		}
	case "enter", "f2":
		return m.replyApproval(p, true, "", false)
	case "backspace", "f3":
		return m.replyApproval(p, false, "", false)
	case "f4":
		return m.action("interrupt", "")
	case "ctrl+r":
		return m.action("resume", "")
	}
	return nil
}
func (m *model) approvalHeight() int {
	if m.selectedApproval() == nil {
		return 0
	}
	return min(9, max(5, m.height/3))
}
func (m *model) approvalBox() string {
	p := m.selectedApproval()
	if p == nil {
		return ""
	}
	s := m.current()
	height := m.approvalHeight()
	index := 0
	for i, v := range s.Pending {
		if v.RequestId == p.RequestId {
			index = i
		}
	}
	rows := []string{warning.Render(fmt.Sprintf("Pending approvals · %d/%d", index+1, len(s.Pending)))}
	capacity := max(1, height-5)
	start := max(0, index-capacity+1)
	for i := start; i < min(len(s.Pending), start+capacity); i++ {
		prefix := "  "
		if i == index {
			prefix = "› "
		}
		rows = append(rows, clip(prefix+pickerLabel(s.Pending[i].Text), max(1, m.width-2)))
	}
	if height >= 6 {
		rows = append(rows, muted.Render(clip("  "+pickerLabel(string(p.Payload)), max(1, m.width-2))))
	}
	footer := "Tab focus · /approval details"
	if m.focusApproval {
		footer = "↑/↓ · Enter allow · Backspace deny"
		if question(p) {
			footer = "/answer required · Backspace deny"
		}
	}
	rows = append(rows, muted.Render(clip(footer, max(1, m.width-2))))
	for len(rows) < height-2 {
		rows = append(rows, "")
	}
	return frame(strings.Join(rows[:min(len(rows), height-2)], "\n"), m.width, m.focusApproval)
}

func (m *model) approvalDetails() {
	p := m.selectedApproval()
	if p == nil {
		m.recordLocal("/approval", "No pending approval")
		return
	}
	var value any
	detail := safeText(string(p.Payload))
	if json.Unmarshal(p.Payload, &value) == nil {
		if b, err := json.MarshalIndent(value, "", "  "); err == nil {
			detail = string(b)
		}
	}
	m.recordLocal("/approval", p.Text+"\n"+detail)
}

func (m *model) fullPermissionNotice() string {
	if s := m.current(); s != nil && s.RunId != "" && m.fullPermission[s.Id] == s.RunId {
		return "FULL permission · /permission ask"
	}
	return ""
}

func localReport(text string, width int) string {
	return indentBlock(ansi.Hardwrap(safeText(text), max(1, width-2), true))
}
