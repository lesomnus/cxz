package multiclient

import (
	"context"
	"testing"

	"github.com/lesomnus/cxz/internal/containerterm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type pathsDaemon struct{ daemon }

func (d *pathsDaemon) Paths(_ context.Context, project, path string, emit func(containerterm.PathListing)) (containerterm.PathListing, error) {
	d.record("paths:" + project + ":" + path)
	out := containerterm.PathListing{Entries: []containerterm.PathEntry{{Name: "remote"}}}
	if emit != nil {
		emit(out)
	}
	return out, nil
}

func TestPathsRouteScopedProjectToItsDaemon(t *testing.T) {
	home, work := &pathsDaemon{}, &pathsDaemon{}
	c := New(context.Background(), []Source{{Name: "home", Client: home}, {Name: "work", Client: work, Remote: true}, {Name: "old", Client: &daemon{}, Remote: true}}, "home")
	defer c.Close()
	ctx := c.ContextFor(context.Background(), "home::project")
	var partial bool
	_, err := c.Paths(ctx, "work::project", "~/한글/", func(containerterm.PathListing) { partial = true })
	if err != nil || !partial {
		t.Fatal(err)
	}
	work.mu.Lock()
	if len(work.calls) != 1 || work.calls[0] != "paths:project:~/한글/" {
		t.Fatal(work.calls)
	}
	work.mu.Unlock()
	if _, err := c.Paths(ctx, "old::project", "/", nil); status.Code(err) != codes.Unimplemented {
		t.Fatal("old client should request upgrade", err)
	}
	if _, err := c.Paths(ctx, "missing::project", "/", nil); err == nil {
		t.Fatal("unknown connection fell back to default")
	}
	home.mu.Lock()
	defer home.mu.Unlock()
	if len(home.calls) != 0 {
		t.Fatal("remote request reached local daemon", home.calls)
	}
}
