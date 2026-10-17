package workspace

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type captureStub struct {
	api.SessionsClient
	sessions []*api.Session
	stall    string
	drained  map[string]bool
}

func (c *captureStub) List(context.Context, *api.Empty, ...grpc.CallOption) (*api.SessionList, error) {
	return &api.SessionList{Sessions: c.sessions}, nil
}
func (c *captureStub) Stop(context.Context, *api.Control, ...grpc.CallOption) (*api.Receipt, error) {
	return &api.Receipt{}, nil
}
func (c *captureStub) History(ctx context.Context, r *api.WatchRequest, _ ...grpc.CallOption) (*api.EventBatch, error) {
	if r.SessionId == c.stall {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	c.drained[r.SessionId] = true
	return &api.EventBatch{}, nil
}

// A session that cannot answer costs its own budget: the remaining sessions are
// still captured, and the project is still removable. A single budget for the
// whole project failed a recreate instead. See cxz#209.
func TestCaptureIsolatesASessionThatDoesNotAnswer(t *testing.T) {
	defer func(b, total time.Duration) { captureBudget, captureTotal = b, total }(captureBudget, captureTotal)
	captureBudget, captureTotal = 50*time.Millisecond, time.Minute

	c := &captureStub{drained: map[string]bool{}, stall: "stuck", sessions: []*api.Session{
		{Id: "stuck", State: "idle"},
		{Id: "live", State: "working"},
		{Id: "gone", State: "stopped"},
	}}
	p := &Project{ID: "p"}
	err := (&Manager{}).capture(context.Background(), p, c)
	if err == nil || !strings.Contains(err.Error(), "session stuck") {
		t.Fatal("unreported stall:", err)
	}
	if !c.drained["live"] || !c.drained["gone"] {
		t.Fatal("a stalled session starved the others", c.drained)
	}
	if len(p.Sessions) != 3 || p.Sessions[0].ProjectId != "p" {
		t.Fatal("sessions were not refreshed", p.Sessions)
	}
}
