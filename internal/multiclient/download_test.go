package multiclient

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"strings"
	"testing"
)

type downloadDaemon struct{ daemon }

func (d *downloadDaemon) Download(_ context.Context, project, path string, dst io.Writer) error {
	d.record("download:" + project + ":" + path)
	_, err := io.WriteString(dst, "remote bytes")
	return err
}
func TestDownloadRoutesToSelectedRemote(t *testing.T) {
	home, work := &downloadDaemon{}, &downloadDaemon{}
	c := New(context.Background(), []Source{{Name: "home", Client: home}, {Name: "work", Client: work, Remote: true}, {Name: "old", Client: &daemon{}, Remote: true}}, "home")
	defer c.Close()
	ctx := c.ContextFor(context.Background(), "home::project")
	var out strings.Builder
	if err := c.Download(ctx, "work::project", "~/한글.zip", &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "remote bytes" {
		t.Fatal(out.String())
	}
	work.mu.Lock()
	if len(work.calls) != 1 || work.calls[0] != "download:project:~/한글.zip" {
		t.Fatal(work.calls)
	}
	work.mu.Unlock()
	if err := c.Download(ctx, "old::project", "/file", &out); status.Code(err) != codes.Unimplemented {
		t.Fatal(err)
	}
	if err := c.Download(ctx, "missing::project", "/file", &out); err == nil {
		t.Fatal("unknown remote accepted")
	}
	home.mu.Lock()
	defer home.mu.Unlock()
	if len(home.calls) != 0 {
		t.Fatal(home.calls)
	}
}
