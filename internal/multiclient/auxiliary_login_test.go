package multiclient

import (
	"context"
	"io"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type auxLoginDaemon struct{ daemon }

func (d *auxLoginDaemon) LoginAuxiliary(_ context.Context, account string, input io.ReadCloser, output io.Writer) error {
	d.record("login:" + account)
	defer input.Close()
	_, err := io.Copy(output, input)
	return err
}

func TestAuxiliaryLoginRoutesToCapturedConnection(t *testing.T) {
	home, work := &auxLoginDaemon{}, &auxLoginDaemon{}
	c := New(context.Background(), []Source{{Name: "home", Client: home}, {Name: "work", Client: work, Remote: true}, {Name: "old", Client: &daemon{}, Remote: true}}, "home")
	defer c.Close()
	ctx := c.ContextFor(context.Background(), "work::project")
	var output strings.Builder
	if err := c.LoginAuxiliary(ctx, "work1", io.NopCloser(strings.NewReader("fixture")), &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "fixture" {
		t.Fatal("login I/O not forwarded")
	}
	work.mu.Lock()
	if len(work.calls) != 1 || work.calls[0] != "login:work1" {
		t.Fatal("scoped project was not resolved", work.calls)
	}
	work.mu.Unlock()
	if err := c.LoginAuxiliary(c.ContextFor(context.Background(), "old::project"), "work1", io.NopCloser(strings.NewReader("")), io.Discard); status.Code(err) != codes.Unimplemented {
		t.Fatal("missing login transport should request upgrade", err)
	}
	home.mu.Lock()
	defer home.mu.Unlock()
	if len(home.calls) != 0 {
		t.Fatal("remote login reached local daemon", home.calls)
	}
}
