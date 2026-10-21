package workspace

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type stopStub struct {
	api.SessionsClient
	sessions []*api.Session
	stall    string
	stopped  map[string]bool
	paged    bool
}

func (c *stopStub) List(context.Context, *api.Empty, ...grpc.CallOption) (*api.SessionList, error) {
	return &api.SessionList{Sessions: c.sessions}, nil
}
func (c *stopStub) Stop(ctx context.Context, r *api.Control, _ ...grpc.CallOption) (*api.Receipt, error) {
	if r.SessionId == c.stall {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	c.stopped[r.SessionId] = true
	return &api.Receipt{}, nil
}
func (c *stopStub) History(context.Context, *api.WatchRequest, ...grpc.CallOption) (*api.EventBatch, error) {
	c.paged = true
	return &api.EventBatch{}, nil
}

// A session that cannot answer costs its own budget: the remaining sessions are
// still stopped, and the project is still removable. A single budget for the
// whole project failed a recreate instead. See cxz#209.
//
// Nothing is paged out of the project any more. The conversation is a file in
// the manager's volume, so taking a project down carries nothing with it --
// which is also why a session that has already stopped is left alone rather
// than asked for its history. See cxz#268.
func TestStoppingSessionsIsolatesOneThatDoesNotAnswerAndPagesNothing(t *testing.T) {
	defer func(b, total time.Duration) { stopBudget, stopTotal = b, total }(stopBudget, stopTotal)
	stopBudget, stopTotal = 50*time.Millisecond, time.Minute

	c := &stopStub{stopped: map[string]bool{}, stall: "stuck", sessions: []*api.Session{
		{Id: "stuck", State: "idle"},
		{Id: "live", State: "working"},
		{Id: "gone", State: "stopped"},
	}}
	p := &Project{ID: "p"}
	err := (&Manager{}).stopSessions(context.Background(), p, c)
	if err == nil || !strings.Contains(err.Error(), "session stuck") {
		t.Fatal("unreported stall:", err)
	}
	if !c.stopped["live"] {
		t.Fatal("a stalled session starved the others", c.stopped)
	}
	if c.stopped["gone"] {
		t.Fatal("a session that had already stopped was stopped again")
	}
	if c.paged {
		t.Fatal("history was paged out of a project whose conversation the manager can read itself")
	}
	if len(p.Sessions) != 3 || p.Sessions[0].ProjectId != "p" {
		t.Fatal("sessions were not refreshed", p.Sessions)
	}
}
