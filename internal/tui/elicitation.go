package tui

import (
	"encoding/json"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

var elicitationActions = []string{"Accept", "Decline", "Cancel"}

func (m *model) elicitationDecision(p *api.Event) bool {
	return p != nil && p.Text == agentview.CodexElicitation && agentview.ElicitationButtons(p.Payload)
}
func (m *model) elicitationIndex(p *api.Event) int {
	s := m.current()
	if s == nil {
		return 0
	}
	key := s.Id + "/" + p.RunId + "/" + p.RequestId
	if key != m.elicitationRequest {
		m.elicitationRequest = key
		m.elicitationChoice = 0
	}
	return m.elicitationChoice
}
func (m *model) decideElicitation(p *api.Event, index int) tea.Cmd {
	if !m.elicitationDecision(p) || index < 0 || index >= len(elicitationActions) {
		return nil
	}
	b, _ := json.Marshal(agentview.ElicitationSelection(elicitationActions[index]))
	return m.replyApproval(p, true, string(b))
}
func (m *model) elicitationKey(p *api.Event, k tea.KeyMsg) (bool, tea.Cmd) {
	if !m.elicitationDecision(p) {
		return false, nil
	}
	if k.Paste {
		return true, nil
	}
	index := m.elicitationIndex(p)
	switch k.String() {
	case "left":
		m.elicitationChoice = (index + 2) % 3
	case "right":
		m.elicitationChoice = (index + 1) % 3
	case "1", "f2":
		return true, m.decideElicitation(p, 0)
	case "2", "f3", "backspace":
		return true, m.decideElicitation(p, 1)
	case "3":
		return true, m.decideElicitation(p, 2)
	case "enter":
		return true, m.decideElicitation(p, index)
	default:
		return false, nil
	}
	return true, nil
}
func elicitationButton(index int) string {
	return "[" + string(rune('1'+index)) + " " + elicitationActions[index] + "]"
}
func (m *model) elicitationButtonRow(p *api.Event) string {
	selected := m.elicitationIndex(p)
	parts := []string{}
	for i := range elicitationActions {
		text := elicitationButton(i)
		if m.focusApproval && i == selected {
			text = focus.Render(text)
		} else {
			text = muted.Render(text)
		}
		parts = append(parts, text)
	}
	return clip("  "+strings.Join(parts, " "), max(1, m.width-2))
}

// Coordinates are local to the conversation, as with other composer panels.
func (m *model) elicitationMouse(v tea.MouseMsg) (bool, tea.Cmd) {
	p := m.selectedApproval()
	if !m.elicitationDecision(p) || m.questionDialog != nil || m.projectView || m.accountView {
		return false, nil
	}
	height := m.approvalHeight()
	if height < 3 || v.Y != m.view.Height+height-1 {
		return false, nil
	}
	if v.Action != tea.MouseActionPress || v.Button != tea.MouseButtonLeft {
		return false, nil
	}
	x := 2
	for i := range elicitationActions {
		width := ansi.StringWidth(elicitationButton(i))
		if v.X >= x && v.X < x+width && v.X < m.width-2 {
			m.focusApproval = true
			m.input.Blur()
			m.elicitationIndex(p)
			m.elicitationChoice = i
			return true, m.decideElicitation(p, i)
		}
		x += width + 1
	}
	return false, nil
}
