package multiclient

import (
	"context"
	"testing"

	"github.com/lesomnus/cxz/internal/containerterm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type terminalDaemon struct{ daemon }

func (d *terminalDaemon) OpenTerminal(_ context.Context, project string, w, h int) (containerterm.Terminal, error) {
	d.record("terminal:" + project)
	return nil, nil
}
func TestTerminalRoutesScopedProject(t *testing.T) {
	home, work := &terminalDaemon{}, &terminalDaemon{}
	c := New(context.Background(), []Source{{Name: "home", Client: home}, {Name: "work", Client: work, Remote: true}, {Name: "old", Client: &daemon{}, Remote: true}}, "home")
	defer c.Close()
	ctx := c.ContextFor(context.Background(), "home::project")
	if _, err := c.OpenTerminal(ctx, "work::project", 80, 24); err != nil {
		t.Fatal(err)
	}
	work.mu.Lock()
	if len(work.calls) != 1 || work.calls[0] != "terminal:project" {
		t.Fatal(work.calls)
	}
	work.mu.Unlock()
	if _, err := c.OpenTerminal(ctx, "old::project", 80, 24); status.Code(err) != codes.Unimplemented {
		t.Fatal(err)
	}
	if _, err := c.OpenTerminal(ctx, "missing::project", 80, 24); err == nil {
		t.Fatal("unknown connection fell back to default")
	}
	home.mu.Lock()
	defer home.mu.Unlock()
	if len(home.calls) != 0 {
		t.Fatal("remote terminal opened on local daemon", home.calls)
	}
}
