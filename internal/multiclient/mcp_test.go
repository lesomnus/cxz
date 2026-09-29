package multiclient

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"google.golang.org/grpc"
	"testing"
)

type mcpDaemon struct {
	daemon
	request mcpconfig.Request
}

func (d *mcpDaemon) Docker(_ context.Context, r *api.DockerInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	json.Unmarshal(r.Spec, &d.request)
	return &api.Receipt{Status: `{"entries":[]}`}, nil
}
func TestMCPRoutesProjectAndRejectsCrossHostSession(t *testing.T) {
	a, b := &mcpDaemon{}, &mcpDaemon{}
	c := New(t.Context(), []Source{{Name: "a", Client: a}, {Name: "b", Client: b}}, "a")
	defer c.Close()
	raw, _ := json.Marshal(mcpconfig.Request{Action: "list", Project: "b::P"})
	req := &api.DockerInput{Action: "mcp", Spec: raw}
	if _, e := c.Docker(t.Context(), req); e != nil {
		t.Fatal(e)
	}
	if b.request.Project != "P" || a.request.Project != "" {
		t.Fatal(a.request, b.request)
	}
	var original mcpconfig.Request
	json.Unmarshal(req.Spec, &original)
	if original.Project != "b::P" {
		t.Fatal("mutated request")
	}
	raw, _ = json.Marshal(mcpconfig.Request{Action: "restart", Project: "P", Session: "b::S"})
	if _, e := c.Docker(c.ContextFor(t.Context(), "a::P"), &api.DockerInput{Action: "mcp", Spec: raw}); e == nil {
		t.Fatal("cross-host session accepted")
	}
}
