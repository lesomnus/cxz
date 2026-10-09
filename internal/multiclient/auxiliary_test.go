package multiclient

import (
	"context"
	"testing"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type auxiliaryDaemon struct {
	daemon
	session string
	calls   int
}

func (d *auxiliaryDaemon) AuxStatus(_ context.Context, r *api.AuxStatusInput, _ ...grpc.CallOption) (*api.AuxState, error) {
	d.calls++
	d.session = r.SessionId
	return &api.AuxState{}, nil
}
func (d *auxiliaryDaemon) AuxConfig(_ context.Context, _ *api.Empty, _ ...grpc.CallOption) (*api.AuxConfigReply, error) {
	d.calls++
	return &api.AuxConfigReply{}, nil
}

// A task belongs to a session, and the session says which manager owns it. The
// connection prefix is the client's own and must not travel, so what arrives
// names the session alone -- and the caller's own copy is left unchanged,
// because it may be retried against another connection.
func TestAuxiliarySessionRoutesToItsManager(t *testing.T) {
	a, b := &auxiliaryDaemon{}, &auxiliaryDaemon{}
	c := New(t.Context(), []Source{{Name: "a", Client: a}, {Name: "b", Client: b}}, "a")
	defer c.Close()
	r := &api.AuxStatusInput{SessionId: "b::S"}
	if _, e := c.AuxStatus(t.Context(), r); e != nil {
		t.Fatal(e)
	}
	if a.calls != 0 || b.session != "S" {
		t.Fatal("wrong manager or session", a.calls, b.session)
	}
	if r.SessionId != "b::S" {
		t.Fatal("mutated input")
	}
	// The configuration belongs to an installation, so it follows the
	// connection the caller is looking at rather than a session of its own.
	if _, e := c.AuxConfig(c.ContextFor(t.Context(), "b::S"), &api.Empty{}); e != nil {
		t.Fatal(e)
	}
	if b.calls != 2 {
		t.Fatal("global settings ignored connection")
	}
}
