package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

func TestAsyncQuestionModalWhileIdle(t *testing.T) {
	m := conversationModel()
	m.current().Agent = "codex"
	p := &api.Event{Kind: "approval", RunId: "run", RequestId: "async:call", Text: agentview.CodexAsyncQuestion, Payload: []byte(`{"item":{"id":"call","type":"agentMessage","delivery":"async","questions":[{"title":"Theme?","options":["Light","Dark"]}]}}`)}
	m.current().Pending = []*api.Event{p}
	m.fullPermission = map[string]string{"s": "run"}
	m.syncQuestion()
	if m.questionDialog == nil || automaticApproval(p) {
		t.Fatal("async question missing or auto-approved")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Theme?") || !strings.Contains(v, "Dark") {
		t.Fatal(v)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("answer command missing")
	}
	m.Update(cmd())
	if m.questionDialog != nil {
		t.Fatal("successful submission did not close dialog")
	}
}
