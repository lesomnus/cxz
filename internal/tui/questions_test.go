package tui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

func questionModel() *model {
	m := conversationModel()
	m.sessions[0].Pending = []*api.Event{{RequestId: "q", RunId: "run", Text: "AskUserQuestion", Payload: []byte(`{"input":{"questions":[{"header":"다음 작업","question":"무엇을?","options":[{"label":"A","preview":"Preview body"},{"label":"B"}]},{"header":"확인 항목","question":"여러 개?","multiSelect":true,"options":[{"label":"X"},{"label":"Y"}]}]}}`)}}
	return m
}
func questionKeyMsg(key string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)} }

func TestInteractiveQuestionAnswers(t *testing.T) {
	m := questionModel()
	m.input.SetValue("draft 한글")
	m.syncQuestion()
	if m.questionDialog == nil {
		t.Fatal("no automatic dialog")
	}
	if got := ansi.Strip(m.sessionScreen()); !strings.Contains(got, "Preview body") || !strings.Contains(got, "╭") {
		t.Fatal(got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.questionDialog.page != 1 {
		t.Fatal("did not advance")
	}
	m.Update(questionKeyMsg(" "))
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(questionKeyMsg(" "))
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(questionKeyMsg("직접 입력"))
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("no reply")
	}
	if _, again := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); again != nil {
		t.Fatal("duplicate submission")
	}
	result := cmd().(approvalResult)
	c := m.client.(*recordingClient)
	if len(c.inputs) != 0 || len(c.answers) != 1 {
		t.Fatal("wrong route")
	}
	a := c.answers[0]
	var values map[string]core.AnswerSelection
	json.Unmarshal([]byte(a.AnswersJson), &values)
	if a.RequestId != "q" || !a.Allow || strings.Join(values["무엇을?"].Selected, ",") != "A" || strings.Join(values["여러 개?"].Selected, ",") != "X,Y" || values["여러 개?"].Other != "직접 입력" {
		t.Fatalf("%+v", a)
	}
	m.Update(result)
	if m.questionDialog != nil || m.input.Value() != "draft 한글" || !m.input.Focused() {
		t.Fatal("composer/dialog state")
	}
}

func TestQuestionCancelReopenStaleAndFailure(t *testing.T) {
	m := questionModel()
	m.syncQuestion()
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.syncQuestion()
	if m.questionDialog != nil {
		t.Fatal("cancel immediately reopened")
	}
	m.input.SetValue("/answer")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.questionDialog == nil || len(m.client.(*recordingClient).inputs) != 0 {
		t.Fatal("answer command not local")
	}
	if m.questionNext() != nil || m.questionDialog.message == "" {
		t.Fatal("empty selection submitted")
	}
	d := m.questionDialog
	d.selected[0][0] = true
	d.selected[1][0] = true
	d.page = 1
	cmd := m.questionNext()
	if cmd == nil {
		t.Fatal("missing submission")
	}
	m.Update(approvalResult{id: "s", run: "run", request: "q", err: errors.New("offline")})
	if m.questionDialog != d || d.sending || !d.selected[0][0] {
		t.Fatal("answer lost on failure")
	}
	m.sessions[0].RunId = "new"
	if m.questionNext() != nil {
		t.Fatal("stale answer submitted")
	}
	m.syncQuestion()
	// Old pending payload is deliberately cleared by the server normally.
	if m.questionDialog != nil && m.questionDialog.run == "run" {
		t.Fatal("stale dialog remains")
	}
}

func TestQuestionPreviewScroll(t *testing.T) {
	m := questionModel()
	m.syncQuestion()
	d := m.questionDialog
	d.questions[0].Options[0].Preview = strings.Repeat("long preview\n", 80) + "LAST PREVIEW LINE"
	m.scrollQuestion(80)
	view := m.questionPanel()
	if len(strings.Split(view, "\n")) != m.questionHeight() || ansi.StringWidth(strings.Split(view, "\n")[0]) > m.width {
		t.Fatal("viewport changed")
	}
	m.scrollQuestion(-10000)
	view = ansi.Strip(m.questionPanel())
	if !strings.Contains(view, "무엇을?") {
		t.Fatal("cannot scroll to beginning")
	}
}

func TestCodexQuestionSecret(t *testing.T) {
	m := conversationModel()
	m.sessions[0].Agent = "codex"
	m.sessions[0].Pending = []*api.Event{{RequestId: "q", RunId: "run", Text: "item/tool/requestUserInput", Payload: []byte(`{"params":{"questions":[{"id":"secret-id","question":"Secret?","isSecret":true}]}}`)}}
	m.syncQuestion()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("private-value"), Paste: true})
	if strings.Contains(ansi.Strip(m.sessionScreen()), "private-value") {
		t.Fatal("secret echoed")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("missing codex reply")
	}
	cmd()
	a := m.client.(*recordingClient).answers[0]
	if a.AnswersJson != `{"secret-id":{"selected":[],"other":"private-value"}}` {
		t.Fatal(a.AnswersJson)
	}
	if m.questionDialog == nil || !m.questionDialog.sending {
		t.Fatal("missing pending send state")
	}
}
