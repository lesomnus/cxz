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
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
)

type approvalResult struct {
	id, run, request string
	err              error
}

func question(p *api.Event) bool          { return core.Question(p.Text) }
func automaticApproval(p *api.Event) bool { return core.AutomaticApproval(p.Text) }
func permissionState(state string) bool {
	return state == "idle" || state == "working" || state == "waiting_input"
}
func (m *model) selectedApproval() *api.Event {
	s := m.current()
	pending := m.visibleApprovals(s)
	if len(pending) == 0 {
		return nil
	}
	for _, p := range pending {
		if p.RequestId == m.approvalID {
			return p
		}
	}
	return pending[0]
}

// Suppress transient requests handled by the supervisor. Only a durable
// approval_resolved event is displayed as an allowed result.
func (m *model) hiddenAutoApproval(s *api.Session, p *api.Event) bool {
	return s != nil && s.RunId != "" && s.PermissionMode == "full" && (p.RunId == "" || p.RunId == s.RunId) && permissionState(s.State) && automaticApproval(p) && !m.projectView && !m.accountView
}
func (m *model) visibleApprovals(s *api.Session) []*api.Event {
	var out []*api.Event
	if s != nil {
		for _, p := range s.Pending {
			if !m.hiddenAutoApproval(s, p) {
				out = append(out, p)
			}
		}
	}
	return out
}
func (m *model) replyApproval(p *api.Event, allow bool, answers string) tea.Cmd {
	s := m.current()
	if s == nil || p == nil {
		m.notice = "No pending approval"
		return nil
	}
	if allow && question(p) && answers == "" {
		return m.openQuestion(p)
	}
	if answers != "" {
		if _, _, err := core.DecodeAnswers(answers); err != nil {
			m.showError(err.Error())
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
		return approvalResult{id: id, run: run, request: request, err: err}
	}
}

type permissionResult struct {
	id, run, mode string
	err           error
}

func (m *model) permissionCommand(text string) tea.Cmd {
	s := m.current()
	if s == nil {
		m.notice = "Select a session first"
		return nil
	}
	parts := strings.Fields(text)
	if len(parts) != 2 || (parts[1] != "full" && parts[1] != "ask") {
		m.recordLocal("/permission", "Usage: /permission full | ask\nThe session supervisor saves this policy and applies it even while this TUI is closed. Questions still require answers.")
		return nil
	}
	if s.RunId == "" || !permissionState(s.State) {
		m.recordLocal("/permission", "Resume the session before changing its permission mode.")
		return nil
	}
	if m.permissionUpdating[s.Id] {
		m.notice = "Permission update pending"
		return nil
	}
	if m.permissionUpdating == nil {
		m.permissionUpdating = map[string]bool{}
	}
	m.permissionUpdating[s.Id] = true
	id, run, mode := s.Id, s.RunId, parts[1]
	requestID := core.ID()
	m.notice = "Saving session permission mode…"
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		receipt, err := m.client.Permission(ctx, &api.PermissionInput{SessionId: id, RunId: run, ClientId: requestID, Mode: mode})
		if err == nil && (receipt == nil || receipt.Status != "accepted") {
			err = fmt.Errorf("permission update was not confirmed")
		}
		return permissionResult{id: id, run: run, mode: mode, err: err}
	}
}
func (m *model) recordLocal(command, text string) {
	if command == "/context" || command == "/usage" || command == "/help" || strings.HasPrefix(command, "/help ") {
		m.openReport(command, text)
		return
	}
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
		return m.confirmInterrupt(time.Now())
	case "pgup", "pgdown", "ctrl+up", "ctrl+down", "ctrl+home", "ctrl+end":
		step := max(1, m.approvalHeight()-5)
		switch k.String() {
		case "pgup":
			m.approvalOffset -= step
		case "pgdown":
			m.approvalOffset += step
		case "ctrl+up":
			m.approvalOffset--
		case "ctrl+down":
			m.approvalOffset++
		case "ctrl+home":
			m.approvalOffset = 0
		case "ctrl+end":
			m.approvalOffset = 1 << 30
		}
		m.approvalOffset = max(0, m.approvalOffset)
	case "up", "down":
		pending := m.visibleApprovals(s)
		for i, v := range pending {
			if v.RequestId == p.RequestId {
				delta := 1
				if k.String() == "up" {
					delta = len(pending) - 1
				}
				m.approvalID = pending[(i+delta)%len(pending)].RequestId
				m.approvalOffset = 0
				break
			}
		}
	case "enter", "f2":
		return m.replyApproval(p, true, "")
	case "backspace", "f3":
		return m.replyApproval(p, false, "")
	case "f4":
		return m.action("interrupt", "")
	case "ctrl+r":
		return m.action("resume", "")
	}
	return nil
}
func (m *model) approvalHeight() int {
	if m.questionDialog != nil {
		return 0
	}
	if m.selectedApproval() == nil {
		return 0
	}
	height := min(12, max(5, m.height/3))
	if m.errorHeight() > 0 {
		height = min(height, max(0, m.height-m.input.Height()-6-m.errorHeight()))
	}
	if height < 3 {
		return 0
	}
	return height
}
func (m *model) approvalBox() string {
	if m.questionDialog != nil {
		return ""
	}
	p := m.selectedApproval()
	if p == nil {
		return ""
	}
	s := m.current()
	height := m.approvalHeight()
	if height < 3 {
		return ""
	}
	index := 0
	pending := m.visibleApprovals(s)
	for i, v := range pending {
		if v.RequestId == p.RequestId {
			index = i
		}
	}
	view := agentview.ApprovalView(s.Agent, p.Text, p.Payload)
	if question(p) {
		if qs, err := agentview.Questions(s.Agent, p.Text, p.Payload); err == nil {
			view.Title = "Question"
			var lines []string
			for _, q := range qs {
				lines = append(lines, q.Text)
			}
			view.Detail = strings.Join(lines, "\n")
		}
	}
	heading := "Pending approvals"
	if question(p) {
		heading = "Pending questions"
	}
	rows := []string{warning.Render(clip(fmt.Sprintf("  %s · %d/%d", heading, index+1, len(pending)), max(1, m.width-2)))}
	rows = append(rows, clip("› "+pickerLabel(view.Title), max(1, m.width-2)))
	capacity := max(0, height-5)
	details := strings.Split(ansi.Hardwrap(safeText(view.Detail), max(1, m.width-4), true), "\n")
	m.approvalOffset = max(0, min(m.approvalOffset, max(0, len(details)-capacity)))
	for i := 0; i < capacity; i++ {
		line := ""
		if m.approvalOffset+i < len(details) {
			line = details[m.approvalOffset+i]
		}
		rows = append(rows, "  "+muted.Render(line))
	}
	footer := "Tab focus · /approval details"
	if m.focusApproval {
		footer = "↑/↓ · Enter allow · Backspace deny · PgUp/PgDn scroll"
		if question(p) {
			footer = "Enter / /answer opens question dialog · Backspace deny"
		}
	}
	if m.focusApproval {
		footer = fmt.Sprintf("L%d/%d · ", m.approvalOffset+1, len(details)) + footer
	}
	rows = append(rows, muted.Render(clip("  "+footer, max(1, m.width-2))))
	for len(rows) < height-2 {
		rows = append(rows, "")
	}
	line := insetRule(m.width, muted)
	if m.focusApproval {
		line = insetRule(m.width, accent)
	}
	return line + "\n" + strings.Join(rows[:min(len(rows), height-2)], "\n") + "\n" + line
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
	if s := m.current(); s != nil && s.PermissionMode == "full" {
		return "FULL"
	}
	return ""
}

func localReport(text string, width int) string {
	return indentBlock(ansi.Hardwrap(safeText(text), max(1, width-2), true))
}
