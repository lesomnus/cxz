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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type deletionFixture struct {
	fixture
	downErr      error
	downs, stops int
}

func (f *deletionFixture) Down(context.Context, *api.ProjectRequest) (*api.Receipt, error) {
	f.downs++
	if f.downErr != nil {
		return nil, f.downErr
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

func TestDeletionTombstonesSurviveSnapshots(t *testing.T) {
	for _, kind := range []string{"session", "project", "project-failure"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			f := &deletionFixture{fixture: fixture{p: &api.Project{Id: "project", Workspace: "/work", Name: "work"}, s: &api.Session{Id: "session", ProjectId: "project", Workspace: "/work", Agent: "claude", Account: "work", AuthBackend: accounts.ProjectLocalOAuth, State: "idle", RunId: "run", CreateId: "create"}}}
			f.s.AuthBinding = accounts.BindingID("project", "work", accounts.ProjectLocalOAuth)
			stack, err := Build(ctx, db, f)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = stack.Session().List(ctx, &resource.SessionListRequest{}); err != nil {
				t.Fatal(err)
			}
			if kind == "project-failure" {
				f.downErr = errors.New("cannot stop owned containers")
				if _, err = stack.Project().Erase(ctx, projectRef("project")); err == nil {
					t.Fatal("ignored cleanup failure")
				}
				ps, err := stack.Project().List(ctx, &resource.ProjectListRequest{})
				if err != nil || len(ps.GetItems()) != 1 {
					t.Fatal("lost project on failed deletion", err)
				}
				return
			}
			if kind == "session" {
				out, err := stack.Session().Erase(ctx, sessionRef("session"))
				if err != nil || !out.GetErased() || f.stops != 1 {
					t.Fatal(out, err, f.stops)
				}
				again, err := stack.Session().Erase(ctx, sessionRef("session"))
				if err != nil || again.GetErased() || f.stops != 1 {
					t.Fatal("non-idempotent deletion", err)
				}
			} else {
				if _, err := stack.Project().Erase(ctx, projectRef("project")); err != nil || f.downs != 1 {
					t.Fatal(err, f.downs)
				}
			}
			for i := 0; i < 2; i++ {
				stack, err = Build(ctx, db, f)
				if err != nil {
					t.Fatal(err)
				}
				ss, err := stack.Session().List(ctx, &resource.SessionListRequest{})
				if err != nil || len(ss.GetItems()) != 0 {
					t.Fatal("session resurrected", ss, err)
				}
				if _, err = stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef("session")}.Build()); status.Code(err) != codes.NotFound {
					t.Fatal("deleted session accessible", err)
				}
				if _, err = stack.Session().Resume(ctx, resource.SessionControl_builder{Ref: sessionRef("session")}.Build()); status.Code(err) != codes.NotFound {
					t.Fatal("deleted session resumed", err)
				}
				ps, err := stack.Project().List(ctx, &resource.ProjectListRequest{})
				if err != nil {
					t.Fatal(err)
				}
				if kind == "project" && len(ps.GetItems()) != 0 {
					t.Fatal("project resurrected")
				}
				if kind == "session" && len(ps.GetItems()) != 1 {
					t.Fatal("session deletion removed project")
				}
			}
			if kind == "project" {
				if _, err = stack.Project().Add(ctx, resource.ProjectAddRequest_builder{Workspace: "/work"}.Build()); err != nil {
					t.Fatal("cannot register removed workspace", err)
				}
				ss, err := stack.Session().List(ctx, &resource.SessionListRequest{})
				if err != nil || len(ss.GetItems()) != 0 {
					t.Fatal("old sessions reappeared", err)
				}
			}
		})
	}
}
