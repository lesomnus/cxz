package multiclient

import (
	"context"
	"github.com/lesomnus/cxz/internal/memorylib"
	"testing"
)

type libraryDaemon struct {
	daemon
	id string
}

func (d *libraryDaemon) Library(_ context.Context, id string, q memorylib.Request) (memorylib.Reply, error) {
	d.id = id
	return memorylib.Reply{Message: q.Action}, nil
}
func TestLibraryRoutesToSourceConnection(t *testing.T) {
	home, work := &libraryDaemon{}, &libraryDaemon{}
	c := New(context.Background(), []Source{{Name: "home", Client: home}, {Name: "work", Client: work, Remote: true}}, "home")
	defer c.Close()
	out, e := c.Library(context.Background(), "work::session", memorylib.Request{Action: "list"})
	if e != nil || out.Message != "list" || work.id != "session" || home.id != "" {
		t.Fatal(out, e, work.id, home.id)
	}
}
