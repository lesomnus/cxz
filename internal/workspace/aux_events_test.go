package workspace

import (
	"context"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"google.golang.org/grpc"
)

// A stream that hands each sent state to the test and ends when the test has
// seen what it asked for.
type auxStateSink struct {
	grpc.ServerStreamingServer[api.AuxState]
	ctx    context.Context
	states chan *api.AuxState
}

func (s *auxStateSink) Context() context.Context { return s.ctx }
func (s *auxStateSink) Send(v *api.AuxState) error {
	select {
	case s.states <- v:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *auxStateSink) next(t *testing.T, why string) *api.AuxState {
	t.Helper()
	select {
	case v := <-s.states:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal(why)
		return nil
	}
}

// The subscription answers immediately with what is already known, and again
// when something changes -- and says nothing when nothing did.
func TestAuxEventsSendsNowAndOnChange(t *testing.T) {
	c, err := auxiliary.New(t.TempDir(), func(context.Context, auxiliary.Input) (auxiliary.Output, error) {
		t.Error("unexpected paid execution")
		return auxiliary.Output{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.Save("summary", auxiliary.Profile{Account: "work", Agent: "codex", Backend: accounts.BrokeredAccessToken, Model: "test"}); err != nil {
		t.Fatal(err)
	}
	m := &Manager{aux: c}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sink := &auxStateSink{ctx: ctx, states: make(chan *api.AuxState, 8)}
	done := make(chan error, 1)
	go func() { done <- m.AuxEvents(&api.AuxStatusInput{SessionId: "s"}, sink) }()

	first := sink.next(t, "nothing was sent for what is already known")
	if len(first.Preferences) == 0 {
		t.Fatal("the first state said nothing about preferences", first)
	}

	// Turning a kind on for this session is a change, so it arrives without
	// anybody asking.
	if _, err = m.AuxPrefer(t.Context(), &api.AuxPreferInput{
		SessionId:   "s",
		Preferences: []*api.AuxPreference{{Kind: "summary", Enabled: true}},
	}); err != nil {
		t.Fatal(err)
	}
	for {
		v := sink.next(t, "a change was not pushed")
		enabled := false
		for _, p := range v.Preferences {
			enabled = enabled || p.Kind == "summary" && p.Enabled
		}
		if enabled {
			break
		}
	}

	// Nothing has changed since, so nothing more is sent.
	select {
	case v := <-sink.states:
		t.Fatal("sent a state that did not change", v)
	case <-time.After(300 * time.Millisecond):
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the subscription outlived its caller")
	}
}

func TestAuxEventsNeedsASession(t *testing.T) {
	c, err := auxiliary.New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	m := &Manager{aux: c}
	if err = m.AuxEvents(&api.AuxStatusInput{}, &auxStateSink{ctx: t.Context(), states: make(chan *api.AuxState, 1)}); err == nil {
		t.Fatal("subscribed to no session")
	}
}
