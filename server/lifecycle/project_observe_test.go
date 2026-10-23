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

// A read answers with what the container is doing now, not with what it was
// doing when something last happened to it. The reconciliation loop runs every
// thirty seconds; a project page that asks is asking now.
func TestAReadObservesTheContainerWithinItsWindow(t *testing.T) {
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &fixture{
		p: &api.Project{Id: "project", Workspace: "/work", Name: "work", State: "running"},
		s: &api.Session{Id: "session", ProjectId: "project", Workspace: "/work", Agent: "claude", Account: "work", AuthBackend: accounts.ProjectLocalOAuth, State: "idle", RunId: "run"},
	}
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	projects := stack.Project()
	state := func() resource.ContainerState {
		t.Helper()
		page, err := projects.List(ctx, resource.ProjectListRequest_builder{}.Build())
		if err != nil {
			t.Fatal(err)
		}
		if len(page.GetItems()) != 1 {
			t.Fatalf("expected the one registered project, got %d", len(page.GetItems()))
		}
		return page.GetItems()[0].GetStatus().GetContainerState()
	}
	if got := state(); got != resource.ContainerState_CONTAINER_STATE_RUNNING {
		t.Fatal(got)
	}
	looks := f.looks
	// Inside the window the answer is shared: polling clients do not each cost
	// a Docker inventory.
	f.p.State = "stopped"
	if got := state(); got != resource.ContainerState_CONTAINER_STATE_RUNNING || f.looks != looks {
		t.Fatal("a second read inside the window asked again", got, f.looks, looks)
	}
	window := observeWindow
	observeWindow = 0
	defer func() { observeWindow = window }()
	if got := state(); got != resource.ContainerState_CONTAINER_STATE_STOPPED {
		t.Fatal("a read outside the window answered with a stale conclusion", got)
	}
	if f.looks <= looks {
		t.Fatal("the container was never looked at again")
	}
	// A look that fails leaves what was known in place rather than reporting an
	// absence, and does not close the window against the next attempt.
	f.lookErr = errors.New("docker daemon unreachable")
	if got := state(); got != resource.ContainerState_CONTAINER_STATE_STOPPED {
		t.Fatal("a failed look overwrote what was known", got)
	}
}

// Stopping keeps the container and the project; starting brings them back. The
// project's identity, its sessions and their conversations are not part of
// either operation -- the resource exists whether or not the container runs.
func TestStopAndStartKeepTheProject(t *testing.T) {
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &fixture{
		p: &api.Project{Id: "project", Workspace: "/work", Name: "work", State: "running"},
		s: &api.Session{Id: "session", ProjectId: "project", Workspace: "/work", Agent: "claude", Account: "work", AuthBackend: accounts.ProjectLocalOAuth, State: "idle", RunId: "run"},
	}
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	projects, sessions := stack.Project(), stack.Session()
	before, err := projects.Get(ctx, resource.ProjectGetRequest_builder{Ref: projectRef("project")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	window := observeWindow
	observeWindow = 0
	defer func() { observeWindow = window }()

	stopped, err := projects.Stop(ctx, resource.ProjectControl_builder{Ref: projectRef("project")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if got := stopped.GetStatus().GetContainerState(); got != resource.ContainerState_CONTAINER_STATE_STOPPED {
		t.Fatal("stop did not answer with what it left behind", got)
	}
	if string(stopped.GetId()) != string(before.GetId()) || stopped.GetAlias() != before.GetAlias() {
		t.Fatal("stop changed the project", stopped)
	}
	page, err := sessions.List(ctx, resource.SessionListRequest_builder{}.Build())
	if err != nil || len(page.GetItems()) != 1 {
		t.Fatal("stop removed the session record", page, err)
	}

	started, err := projects.Start(ctx, resource.ProjectStartRequest_builder{Ref: projectRef("project")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if got := started.GetStatus().GetContainerState(); got != resource.ContainerState_CONTAINER_STATE_RUNNING {
		t.Fatal("start did not answer with a running container", got)
	}
	if string(started.GetId()) != string(before.GetId()) {
		t.Fatal("start replaced the project", started)
	}
}
