package tui

import (
	"context"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"strings"
	"testing"
)

type recordingClient struct {
	api.SessionsClient
	inputs     []*api.Input
	answers    []*api.Answer
	interrupts []*api.Control
}

func TestDiagnosticAndAuthenticationHint(t *testing.T) {
	s := &api.Session{Id: "s", Agent: "codex", ProjectId: "project", Account: "work-codex"}
	e := &api.Event{Kind: "diagnostic", Text: "authentication failed", Payload: []byte(`{"code":401}`)}
	m := &model{sessions: []*api.Session{s}, view: viewport.New(120, 20), events: map[string][]*api.Event{"s": {e}}}
	m.render()
	text := m.view.View()
	if !strings.Contains(text, "authentication failed") || !strings.Contains(text, "cxz account login --project project work-codex") {
		t.Fatal(text)
	}
	if authHint(s, &api.Event{Kind: "assistant", Text: "401"}) != "" {
		t.Fatal("assistant text misclassified")
	}
}

func (c *recordingClient) Send(_ context.Context, r *api.Input, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.inputs = append(c.inputs, r)
	return &api.Receipt{Status: "accepted"}, nil
}
func (c *recordingClient) Reply(_ context.Context, r *api.Answer, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.answers = append(c.answers, r)
	return &api.Receipt{Status: "accepted"}, nil
}
func (c *recordingClient) Interrupt(_ context.Context, r *api.Control, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.interrupts = append(c.interrupts, r)
	return &api.Receipt{Status: "accepted"}, nil
}
func TestKeyboardControls(t *testing.T) {
	c := &recordingClient{}
	input := textarea.New()
	input.Focus()
	m := &model{ctx: context.Background(), client: c, input: input, view: viewport.New(60, 8), watchID: "s", sessions: []*api.Session{{Id: "s", RunId: "run", Pending: []*api.Event{{RequestId: "permission"}}}}, events: map[string][]*api.Event{}, cursor: map[string]uint64{}}
	m.input.SetValue("hello")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("missing send")
	}
	cmd()
	if len(c.inputs) != 1 || c.inputs[0].Text != "hello" || c.inputs[0].RunId != "run" {
		t.Fatal("bad send")
	}
	for _, k := range []tea.KeyType{tea.KeyF2, tea.KeyF3, tea.KeyF4} {
		if k == tea.KeyF3 {
			m.sessions[0].Pending = []*api.Event{{RequestId: "permission-deny"}}
		}
		_, cmd = m.Update(tea.KeyMsg{Type: k})
		if cmd == nil {
			t.Fatal("missing action")
		}
		cmd()
	}
	if len(c.answers) != 2 || !c.answers[0].Allow || c.answers[1].Allow || c.answers[0].RequestId != "permission" || len(c.interrupts) != 1 {
		t.Fatal("bad approval/interrupt mapping")
	}
	m.input.SetValue(`/answer {"question":"Blue"}`)
	m.sessions[0].Pending = []*api.Event{{RequestId: "question", Text: "AskUserQuestion"}}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	cmd()
	if c.answers[2].AnswersJson != `{"question":"Blue"}` {
		t.Fatal("lost question answer")
	}
	m.Update(disconnected{id: "s"})
	if len(c.inputs) != 1 {
		t.Fatal("replayed input on disconnection")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m.Update(accountListing{accounts: []*resource.Account{resource.Account_builder{Alias: "personal", Agent: "claude"}.Build(), resource.Account_builder{Alias: "work", Agent: "codex"}.Build()}})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.accounts[m.accountIndex].GetAlias() != "work" || !m.creating || !strings.Contains(m.notice, "work · codex") {
		t.Fatal("account selection lost")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("quit must detach")
	}
}
func TestTerminalOutputAndReplay(t *testing.T) {
	if got := safeText("before\x1b]52;c;YWJj\a\x1b[2Jafter"); got != "beforeafter" {
		t.Fatalf("terminal injection: %q", got)
	}
	m := &model{sessions: []*api.Session{{Id: "s"}}, view: viewport.New(30, 5), events: map[string][]*api.Event{}, cursor: map[string]uint64{}}
	for range 2 {
		m.Update(received{id: "s", event: &api.Event{Seq: 1, Kind: "assistant", Text: "hello"}})
	}
	if len(m.events["s"]) != 1 || !strings.Contains(m.view.View(), "hello") {
		t.Fatal("duplicate replay or missing text")
	}
}
