package multiclient

import (
	"context"
	"io"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type loginDaemon struct{ daemon }

func (d *loginDaemon) LoginSession(_ context.Context, project, account, key string, input io.ReadCloser, output io.Writer) error {
	d.record("login:" + project + ":" + account + ":" + key)
	defer input.Close()
	_, err := io.Copy(output, input)
	return err
}

func TestSessionLoginRoutesToCapturedConnection(t *testing.T) {
	home, work := &loginDaemon{}, &loginDaemon{}
	c := New(context.Background(), []Source{{Name: "home", Client: home}, {Name: "work", Client: work, Remote: true}, {Name: "old", Client: &daemon{}, Remote: true}}, "home")
	defer c.Close()
	ctx := c.ContextFor(context.Background(), "home::project")
	var output strings.Builder
	if err := c.LoginSession(ctx, "work::project", "work1", "creation", io.NopCloser(strings.NewReader("fixture")), &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "fixture" {
		t.Fatal("login I/O not forwarded")
	}
	work.mu.Lock()
	if len(work.calls) != 1 || work.calls[0] != "login:project:work1:creation" {
		t.Fatal("scoped project was not resolved", work.calls)
	}
	work.mu.Unlock()
	if err := c.LoginSession(ctx, "old::project", "work1", "creation", io.NopCloser(strings.NewReader("")), io.Discard); status.Code(err) != codes.Unimplemented {
		t.Fatal("missing login transport should request upgrade", err)
	}
	home.mu.Lock()
	defer home.mu.Unlock()
	if len(home.calls) != 0 {
		t.Fatal("remote login reached local daemon", home.calls)
	}
}
