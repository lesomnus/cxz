package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"strings"
	"testing"
)

func TestMCPElicitationVisibleInFullModeAndDismissWithoutFields(t *testing.T) {
	m := questionModel()
	s := m.current()
	s.Agent = "codex"
	s.PermissionMode = "full"
	s.State = "waiting_input"
	s.Pending = []*api.Event{{RunId: s.RunId, RequestId: "mcp", Text: agentview.CodexElicitation, Payload: []byte(`{"params":{"serverName":"cxz_memory","mode":"form","message":"Save?","requestedSchema":{"type":"object","properties":{"note":{"type":"string"}},"required":["note"]}}}`)}}
	if len(m.visibleApprovals(s)) != 1 {
		t.Fatal("MCP input hidden by full mode")
	}
	m.openQuestion(s.Pending[0])
	d := m.questionDialog
	if d == nil || len(d.questions) != 2 {
		t.Fatal("missing MCP form")
	}
	if v := ansi.Strip(m.sessionScreen()); !strings.Contains(v, "cxz_memory") || !strings.Contains(v, "Save?") {
		t.Fatal(v)
	}
	d.selected[0][2] = true
	if cmd := m.questionNext(); cmd == nil || !d.sending || d.page != 0 {
		t.Fatal("cancel demanded form fields")
	}
}
func TestMCPElicitationOptionalFieldCanAdvance(t *testing.T) {
	m := questionModel()
	s := m.current()
	s.Agent = "codex"
	s.Pending = []*api.Event{{RunId: s.RunId, RequestId: "mcp", Text: agentview.CodexElicitation, Payload: []byte(`{"params":{"serverName":"test","mode":"form","message":"Input","requestedSchema":{"type":"object","properties":{"note":{"type":"string"}}}}}`)}}
	m.openQuestion(s.Pending[0])
	d := m.questionDialog
	d.selected[0][0] = true
	m.questionNext()
	if d.page != 1 {
		t.Fatal("accept did not advance")
	}
	if cmd := m.questionNext(); cmd == nil || !d.sending {
		t.Fatal("blank optional input rejected")
	}
}

func confirmationModel(mode string) *model {
	m := questionModel()
	s := m.current()
	s.Agent = "codex"
	s.PermissionMode = mode
	s.State = "waiting_input"
	s.Pending = []*api.Event{{RunId: s.RunId, RequestId: "mcp", Text: agentview.CodexElicitation, Payload: []byte(`{"params":{"serverName":"cxz_memory","mode":"form","message":"Allow memory_list?","requestedSchema":{"type":"object","properties":{}}}}`)}}
	m.resize()
	m.syncQuestion()
	return m
}
func TestMCPConfirmationFullHiddenAndAskHasThreeButtons(t *testing.T) {
	m := confirmationModel("full")
	if m.questionDialog != nil || len(m.visibleApprovals(m.current())) != 0 || m.approvalBox() != "" {
		t.Fatal("full confirmation flashed UI")
	}
	m = confirmationModel("ask")
	if m.questionDialog != nil {
		t.Fatal("confirmation opened question form")
	}
	box := ansi.Strip(m.approvalBox())
	for _, label := range []string{"[1 Accept]", "[2 Decline]", "[3 Cancel]"} {
		if !strings.Contains(box, label) {
			t.Fatal(box)
		}
	}
	if strings.Contains(box, "Question") || strings.Contains(box, "Submit") {
		t.Fatal(box)
	}
}
func TestMCPConfirmationButtonsSendImmediately(t *testing.T) {
	for i, action := range elicitationActions {
		t.Run(action, func(t *testing.T) {
			m := confirmationModel("ask")
			x := 2
			for previous := 0; previous < i; previous++ {
				x += ansi.StringWidth(elicitationButton(previous)) + 1
			}
			handled, cmd := m.elicitationMouse(tea.MouseMsg{X: x + 1, Y: m.view.Height + m.approvalHeight() - 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			if !handled || cmd == nil || m.questionDialog != nil {
				t.Fatal("click required another submit")
			}
			cmd()
			c := m.client.(*recordingClient)
			if len(c.answers) != 1 {
				t.Fatal(c.answers)
			}
			_, selections, err := core.DecodeAnswers(c.answers[0].AnswersJson)
			if err != nil || selections["cxz:action"].Selected[0] != action {
				t.Fatal(c.answers, err)
			}
			if cmd := m.decideElicitation(m.selectedApproval(), i); cmd != nil {
				t.Fatal("duplicate reply sent")
			}
		})
	}
}
func TestMCPConfirmationKeyboardAndURL(t *testing.T) {
	m := confirmationModel("ask")
	m.focusApproval = true
	m.approvalKey(tea.KeyMsg{Type: tea.KeyRight})
	cmd := m.approvalKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || m.questionDialog != nil {
		t.Fatal("keyboard required submit")
	}
	cmd()
	if !strings.Contains(m.client.(*recordingClient).answers[0].AnswersJson, "Decline") {
		t.Fatal("wrong selected button")
	}
	m = confirmationModel("full")
	m.current().Pending[0].Payload = []byte(`{"params":{"serverName":"external","mode":"url","message":"Complete login","url":"https://example.com/auth"}}`)
	m.resize()
	m.syncQuestion()
	if len(m.visibleApprovals(m.current())) != 1 || m.questionDialog != nil || !strings.Contains(ansi.Strip(m.approvalBox()), "Accept") {
		t.Fatal("URL confirmation hidden or made a question")
	}
}

func TestMCPConfirmationMouseThroughUpdate(t *testing.T) {
	m := confirmationModel("ask")
	row, column := -1, -1
	for y, line := range strings.Split(ansi.Strip(m.sessionScreen()), "\n") {
		if x := strings.Index(line, "[1 Accept]"); x >= 0 {
			row, column = y, x+2
			break
		}
	}
	if row < 0 {
		t.Fatal("no rendered button")
	}
	_, cmd := m.update(tea.MouseMsg{X: m.contentOffset() + column, Y: row, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if cmd == nil {
		t.Fatal("mouse event intercepted before approval button")
	}
	cmd()
	if len(m.client.(*recordingClient).answers) != 1 || m.questionDialog != nil {
		t.Fatal("click did not reply directly")
	}
}
