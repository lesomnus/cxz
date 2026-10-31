package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type deletionFixture struct {
	fixture
	removeErr       error
	removals, stops int
	erasures        int
	erasedSessions  []string
	erased          bool
}

// RemoveProject is the manager's half of erasing a project: the containers, the
// owned volumes and state, and every session's data. A manager that has erased
// a project stops reporting it, which is what keeps a later projection from
// putting the rows back.
func (f *deletionFixture) RemoveProject(_ context.Context, id string, sessions []string) error {
	f.erasures++
	f.erasedSessions = sessions
	if f.removeErr != nil {
		return f.removeErr
	}
	f.erased = true
	return nil
}

func (f *deletionFixture) ResourceSnapshot(ctx context.Context) (*api.ProjectList, *api.SessionList, error) {
	if f.erased {
		return &api.ProjectList{}, &api.SessionList{}, nil
	}
	return f.fixture.ResourceSnapshot(ctx)
}

func (f *deletionFixture) RemoveContainer(context.Context, *api.ProjectRequest) (*api.Receipt, error) {
	f.removals++
	if f.removeErr != nil {
		return nil, f.removeErr
	}
	f.s.State = "stopped"
	return &api.Receipt{}, nil
}
func (f *deletionFixture) Stop(context.Context, *api.Control) (*api.Receipt, error) {
	f.stops++
	f.s.State = "stopped"
	return &api.Receipt{}, nil
}
func (f *deletionFixture) Get(context.Context, *api.SessionRef) (*api.Session, error) {
	return f.s, nil
}

func TestProjectEraseSurvivesSnapshots(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			ctx := t.Context()
			db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			f := &deletionFixture{fixture: fixture{p: &api.Project{Id: "project", Workspace: "/work", Name: "work"}, s: &api.Session{Id: "session", ProjectId: "project", Workspace: "/work", Agent: "claude", Account: "work", AuthBackend: accounts.ProjectLocalOAuth, State: "idle", RunId: "run", CreateId: "create"}}}
			f.s.AuthBinding = accounts.BindingID("project", "work", accounts.ProjectLocalOAuth)
			stack, err := Build(ctx, db, f, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = stack.Session().List(ctx, &resource.SessionListRequest{}); err != nil {
				t.Fatal(err)
			}
			if fail {
				f.removeErr = errors.New("cannot stop owned containers")
			}
			_, err = stack.Project().Erase(ctx, projectRef("project"))
			if fail {
				if err == nil {
					t.Fatal("ignored cleanup failure")
				}
				ps, e := stack.Project().List(ctx, &resource.ProjectListRequest{})
				if e != nil || len(ps.GetItems()) != 1 {
					t.Fatal("lost project after failed cleanup", e)
				}
				return
			}
			if err != nil || f.erasures != 1 {
				t.Fatal(err, f.erasures)
			}
			for i := 0; i < 2; i++ {
				stack, err = Build(ctx, db, f, nil)
				if err != nil {
					t.Fatal(err)
				}
				ss, e := stack.Session().List(ctx, &resource.SessionListRequest{})
				if e != nil || len(ss.GetItems()) != 0 {
					t.Fatal("sessions resurrected", e)
				}
				ps, e := stack.Project().List(ctx, &resource.ProjectListRequest{})
				if e != nil || len(ps.GetItems()) != 0 {
					t.Fatal("project resurrected", e)
				}
			}
			if _, err = stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef("session")}.Build()); status.Code(err) != codes.NotFound {
				t.Fatal("deleted session accessible", err)
			}
		})
	}
}
