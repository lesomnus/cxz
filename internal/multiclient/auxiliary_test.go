package multiclient

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"google.golang.org/grpc"
	"testing"
)

type auxiliaryDaemon struct {
	daemon
	request auxiliary.Request
	calls   int
}

func (d *auxiliaryDaemon) Docker(_ context.Context, r *api.DockerInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	d.calls++
	_ = json.Unmarshal(r.Spec, &d.request)
	return &api.Receipt{Status: `{}`}, nil
}
func TestAuxiliarySessionRoutesToItsManager(t *testing.T) {
	a, b := &auxiliaryDaemon{}, &auxiliaryDaemon{}
	c := New(t.Context(), []Source{{Name: "a", Client: a}, {Name: "b", Client: b}}, "a")
	defer c.Close()
	raw, _ := json.Marshal(auxiliary.Request{Action: "status", Session: "b::S"})
	r := &api.DockerInput{Action: "auxiliary", Spec: raw}
	if _, e := c.Docker(t.Context(), r); e != nil {
		t.Fatal(e)
	}
	if a.calls != 0 || b.request.Session != "S" {
		t.Fatal("wrong manager or session", a.calls, b.request)
	}
	var original auxiliary.Request
	_ = json.Unmarshal(r.Spec, &original)
	if original.Session != "b::S" {
		t.Fatal("mutated input")
	}
	raw, _ = json.Marshal(auxiliary.Request{Action: "list"})
	if _, e := c.Docker(c.ContextFor(t.Context(), "b::S"), &api.DockerInput{Action: "auxiliary", Spec: raw}); e != nil {
		t.Fatal(e)
	}
	if b.calls != 2 {
		t.Fatal("global settings ignored connection")
	}
}
