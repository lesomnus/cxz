package tui

import (
	"context"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/bed"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"testing"
)

type recordingClient struct {
	api.SessionsClient
	inputs      []*api.Input
	answers     []*api.Answer
	interrupts  []*api.Control
	permissions []*api.PermissionInput
	// What the server says about a send: "queued" when it is holding the
	// message, or an error when it is already holding one.
	status  string
	sendErr error
	// journal is what EventDetails answers from, the way a runtime does. A
	// session's loaded records and the journal behind them are the same data,
	// so the fixture points this at the model rather than copying it.
	journal    func() []*api.Event
	detailsErr error
}

// EventDetails answers with the records sharing the subject's request
// identity, which is what the runtime returns: a tool's call, its output and
// its result, plus the approval it waited on. A row does not carry payloads,
// so this is where a detail panel gets them.
func (c *recordingClient) EventDetails(_ context.Context, r *api.EventDetailsRequest, _ ...grpc.CallOption) (*api.EventBatch, error) {
	if c.detailsErr != nil {
		return nil, c.detailsErr
	}
	var journal []*api.Event
	if c.journal != nil {
		journal = c.journal()
	}
	var subject *api.Event
	for _, e := range journal {
		if e.Seq == r.Seq {
			subject = e
		}
	}
	if subject == nil {
		return nil, status.Error(codes.NotFound, "event is no longer retained")
	}
	out := &api.EventBatch{Events: []*api.Event{subject}}
	for _, e := range journal {
		if e.Seq != subject.Seq && e.RunId == subject.RunId && e.RequestId != "" && e.RequestId == subject.RequestId {
			out.Events = append(out.Events, e)
		}
	}
	return out, nil
}

func TestDiagnosticAndAuthenticationHint(t *testing.T) {
	s := &api.Session{Id: "s", Agent: "codex", ProjectId: "project", Account: "work-codex"}
	e := &api.Event{Seq: 1, Kind: "diagnostic", Text: "authentication failed", Payload: []byte(`{"code":401}`)}
	m := &model{sessions: []*api.Session{s}, view: viewport.New(120, 20), events: map[string][]*api.Event{"s": {e}}, rows: map[string][]*api.Event{"s": rowsOf([]*api.Event{e})}}
	m.render()
	text := m.view.View()
	if !strings.Contains(text, "authentication failed") || !strings.Contains(text, "cxz account login --session s work-codex") {
		t.Fatal(text)
	}
	if authHint(s, &api.Event{Kind: "assistant", Text: "401"}) != "" {
		t.Fatal("assistant text misclassified")
	}
}

func (c *recordingClient) Send(_ context.Context, r *api.Input, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.inputs = append(c.inputs, r)
	if c.sendErr != nil {
		return nil, c.sendErr
	}
	status := c.status
	if status == "" {
		status = "accepted"
	}
	return &api.Receipt{Status: status}, nil
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
	input := bed.New()
	input.Focus()
	m := &model{ctx: context.Background(), client: c, input: input, view: viewport.New(60, 8), watchID: "s", sessions: []*api.Session{{Id: "s", RunId: "run", Pending: []*api.Event{{RequestId: "permission"}}}}, events: map[string][]*api.Event{}, cursor: map[string]uint64{}}
	m.input.SetValue("hello")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("missing send")
	}
	cmd()
	if len(c.inputs) != 1 || c.inputs[0].Text != "hello" || c.inputs[0].RunId != "run" {
		t.Fatal("bad send")
	}
	for _, k := range []tea.KeyType{tea.KeyEnter, tea.KeyBackspace, tea.KeyF4} {
		m.focusApproval = true
		if k == tea.KeyBackspace {
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
	m.focusApproval = false
	m.input.SetValue(`/answer {"question":"Blue"}`)
	m.sessions[0].Pending = []*api.Event{{RequestId: "question", Text: "AskUserQuestion"}}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	cmd()
	if c.answers[2].AnswersJson != `{"question":"Blue"}` {
		t.Fatal("lost question answer")
	}
	m.Update(disconnected{id: "s"})
	if len(c.inputs) != 1 {
		t.Fatal("replayed input on disconnection")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m.Update(accountListing{request: m.accountRequest, accounts: []*resource.Account{resource.Account_builder{Alias: "personal", Agent: "claude"}.Build(), resource.Account_builder{Alias: "work", Agent: "codex"}.Build()}})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.accounts[m.accountIndex].GetAlias() != "work" || !m.creating || !strings.Contains(m.notice, "work · codex") {
		t.Fatal("account selection lost")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("quit must detach")
	}
}
func TestTerminalOutputAndReplay(t *testing.T) {
	if got := safeText("before\x1b]52;c;YWJj\a\x1b[2Jafter"); got != "beforeafter" {
		t.Fatalf("terminal injection: %q", got)
	}
	m := &model{input: newComposer(), sessions: []*api.Session{{Id: "s"}}, view: viewport.New(30, 5), events: map[string][]*api.Event{}, cursor: map[string]uint64{}}
	for range 2 {
		live(m, "s", &api.Event{Seq: 1, Kind: "assistant", Text: "hello"})
	}
	if len(m.events["s"]) != 1 || !strings.Contains(m.view.View(), "hello") {
		t.Fatal("duplicate replay or missing text")
	}
}

func (c *recordingClient) Permission(_ context.Context, r *api.PermissionInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.permissions = append(c.permissions, r)
	return &api.Receipt{ClientId: r.ClientId, Status: "accepted"}, nil
}
