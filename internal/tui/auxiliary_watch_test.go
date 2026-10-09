package tui

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type pushClient struct {
	api.SessionsClient
	states  []*api.AuxState
	opens   int
	err     error
	session string
}

func (c *pushClient) AuxEvents(_ context.Context, in *api.AuxStatusInput, _ ...grpc.CallOption) (grpc.ServerStreamingClient[api.AuxState], error) {
	c.opens++
	c.session = in.SessionId
	if c.err != nil {
		return nil, c.err
	}
	return &pushStream{states: c.states}, nil
}

// AuxStatus is here to fail the test if anything still polls.
func (c *pushClient) AuxStatus(context.Context, *api.AuxStatusInput, ...grpc.CallOption) (*api.AuxState, error) {
	return nil, status.Error(codes.Unimplemented, "the transcript should be subscribed, not asking")
}

type pushStream struct {
	grpc.ClientStream
	states []*api.AuxState
}

func (s *pushStream) Recv() (*api.AuxState, error) {
	if len(s.states) == 0 {
		return nil, io.EOF
	}
	out := s.states[0]
	s.states = s.states[1:]
	return out, nil
}

func pushModel(t *testing.T, c *pushClient) *model {
	t.Helper()
	m := conversationModel()
	m.ctx = context.Background()
	m.client = c
	m.events["s"] = []*api.Event{
		{SessionId: "s", RunId: "run", Seq: 1, Kind: "input", Text: "question"},
		{SessionId: "s", RunId: "run", Seq: 2, Kind: "assistant", Text: "final answer"},
		{SessionId: "s", RunId: "run", Seq: 3, Kind: "turn_end", Text: "completed"},
	}
	m.render()
	return m
}

// One subscription per open conversation, and what it pushes is drawn. Nothing
// asks: the fixture's unary call refuses, so a poll would fail the test.
func TestAuxiliaryStateIsPushedNotPolled(t *testing.T) {
	c := &pushClient{states: []*api.AuxState{{
		Summaries: []*api.AuxSummary{{RunId: "run", Turn: 3, Text: "what happened"}},
		Current:   &api.Aux{Id: "j", RunId: "run", Turn: 3, State: "completed", Kinds: []string{"summary"}},
	}}}
	m := pushModel(t, c)
	open := m.watchAuxiliary()
	if open == nil {
		t.Fatal("no subscription for the open conversation")
	}
	_, cmd := m.Update(open())
	if c.opens != 1 || c.session != "s" {
		t.Fatal("wrong subscription", c.opens, c.session)
	}
	if cmd == nil {
		t.Fatal("the stream was not read")
	}
	if _, next := m.Update(cmd()); next == nil {
		t.Fatal("the stream was not read again")
	}
	if !strings.Contains(ansi.Strip(m.conversationView()), "what happened") {
		t.Fatal("the pushed summary is not drawn")
	}
	// A second pass keeps the one subscription it has rather than opening more.
	if m.watchAuxiliary() != nil || c.opens != 1 {
		t.Fatal("resubscribed while subscribed", c.opens)
	}
}

// A subscription that cannot be opened is reported and retried later, not
// retried immediately and not silently forgotten.
func TestAuxiliarySubscriptionFailureIsHeldOff(t *testing.T) {
	c := &pushClient{err: status.Error(codes.Unavailable, "no manager")}
	m := pushModel(t, c)
	open := m.watchAuxiliary()
	if open == nil {
		t.Fatal("no attempt")
	}
	m.Update(open())
	if m.auxiliaryError == "" {
		t.Fatal("a failure was not reported")
	}
	if m.watchAuxiliary() != nil || c.opens != 1 {
		t.Fatal("retried without waiting", c.opens)
	}
}

// The subscription follows the open conversation: leaving one ends its stream.
func TestAuxiliarySubscriptionFollowsTheConversation(t *testing.T) {
	c := &pushClient{states: []*api.AuxState{{}}}
	m := pushModel(t, c)
	m.Update(m.watchAuxiliary()())
	key := m.connectionRef() + "/s"
	w := m.auxiliaryWatches[key]
	if w == nil || !w.open {
		t.Fatal("not subscribed")
	}
	m.sessions = nil
	m.selected = 0
	if cmd := m.watchAuxiliary(); cmd != nil {
		t.Fatal("subscribed with no conversation open")
	}
	if m.auxiliaryWatches[key] != nil {
		t.Fatal("the stream outlived the conversation")
	}
}
