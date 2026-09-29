package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
)

type removeFixture struct {
	fixture
	removed bool
	calls   int
	ids     []string
	err     error
}

func (f *removeFixture) RemoveProject(_ context.Context, id string, ids []string) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	if id != f.p.Id {
		return errors.New("wrong project")
	}
	f.ids = ids
	f.removed = true
	return nil
}
func (f *removeFixture) ResourceSnapshot(ctx context.Context) (*api.ProjectList, *api.SessionList, error) {
	if f.removed {
		return &api.ProjectList{}, &api.SessionList{}, nil
	}
	return f.fixture.ResourceSnapshot(ctx)
}
func TestProjectRemovePreviewFailureRetryAndRegistry(t *testing.T) {
	ctx := t.Context()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &removeFixture{fixture: fixture{p: &api.Project{Id: "project", Workspace: "/work", Name: "work"}, s: &api.Session{Id: "session", ProjectId: "project", Workspace: "/work", Agent: "claude", Account: "work", AuthBackend: accounts.ProjectLocalOAuth, State: "stopped", CreateId: "create"}}}
	f.s.AuthBinding = accounts.BindingID("project", "work", accounts.ProjectLocalOAuth)
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	out, err := stack.Project().Remove(ctx, resource.ProjectRemoveRequest_builder{Target: ptr("/work")}.Build())
	if err != nil || out.GetRemoved() || out.GetProject().GetRuntimeId() != "project" || f.calls != 0 {
		t.Fatal(out, err, f.calls)
	}
	// Hidden sessions must be included in permanent deletion.
	if _, err := stack.Session().Erase(ctx, sessionRef("session")); err != nil {
		t.Fatal(err)
	}
	f.err = errors.New("volume is in use")
	request := resource.ProjectRemoveRequest_builder{Target: ptr("project"), Confirmed: ptr(true)}.Build()
	if _, err := stack.Project().Remove(ctx, request); err == nil {
		t.Fatal("cleanup error ignored")
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM project").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	f.err = nil
	out, err = stack.Project().Remove(ctx, request)
	if err != nil || !out.GetRemoved() || len(f.ids) != 1 || f.ids[0] != "session" {
		t.Fatal(out, err, f.ids)
	}
	for _, table := range []string{"project", "session", "authbinding"} {
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	if err := db.QueryRow("SELECT count(*) FROM account").Scan(&count); err != nil || count != 1 {
		t.Fatal("shared account removed", count, err)
	}
	// A fresh registry projection cannot resurrect the project.
	stack, err = Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	list, err := stack.Project().List(ctx, &resource.ProjectListRequest{})
	if err != nil || len(list.GetItems()) != 0 {
		t.Fatal(list, err)
	}
}
