package multiclient

import (
	"context"
	"testing"
)

type renameProjectDaemon struct {
	daemon
	id, title string
}

func (d *renameProjectDaemon) RenameProject(_ context.Context, id, title string) error {
	d.id, d.title = id, title
	return nil
}
func TestProjectTitleRoutesToSelectedRemote(t *testing.T) {
	home, work := &renameProjectDaemon{}, &renameProjectDaemon{}
	c := New(context.Background(), []Source{{Name: "home", Client: home}, {Name: "work", Client: work, Remote: true}}, "home")
	defer c.Close()
	if err := c.RenameProject(c.ContextFor(context.Background(), "home::p"), "work::p", "New title"); err != nil {
		t.Fatal(err)
	}
	if work.id != "p" || work.title != "New title" || home.id != "" {
		t.Fatal("rename routed to wrong project")
	}
	if err := c.RenameProject(context.Background(), "missing::p", "title"); err == nil {
		t.Fatal("unknown connection fell back")
	}
}
