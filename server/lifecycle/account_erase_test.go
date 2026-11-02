package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type accountEraseRuntime struct {
	*fixture
	projects                  []*api.Project
	sessions                  []*api.Session
	purged, stopped, accounts []string
	failSession               string
	cleanupErr, authorityErr  error
}

func (f *accountEraseRuntime) ResourceSnapshot(context.Context) (*api.ProjectList, *api.SessionList, error) {
	return &api.ProjectList{Projects: f.projects}, &api.SessionList{Sessions: f.sessions}, nil
}
func (f *accountEraseRuntime) AccountEraseAllowed(context.Context) error { return f.authorityErr }
func (f *accountEraseRuntime) Get(_ context.Context, ref *api.SessionRef) (*api.Session, error) {
	for _, s := range f.sessions {
		if s.Id == ref.Id {
			return s, nil
		}
	}
	return nil, status.Error(codes.NotFound, "session not found")
}
func (f *accountEraseRuntime) Stop(_ context.Context, in *api.Control) (*api.Receipt, error) {
	for _, s := range f.sessions {
		if s.Id == in.SessionId {
			s.State = "stopped"
			f.stopped = append(f.stopped, s.Id)
			return &api.Receipt{}, nil
		}
	}
	return nil, status.Error(codes.NotFound, "session not found")
}
func (f *accountEraseRuntime) EraseSession(_ context.Context, in *api.SessionEraseInput) (*api.SessionEraseReply, error) {
	if in.SessionId == f.failSession {
		return nil, errors.New("session cleanup failed")
	}
	for i, s := range f.sessions {
		if s.Id == in.SessionId {
			f.sessions = append(f.sessions[:i], f.sessions[i+1:]...)
			break
		}
	}
	f.purged = append(f.purged, in.SessionId)
	return &api.SessionEraseReply{SessionId: in.SessionId}, nil
}
func (f *accountEraseRuntime) EraseAccount(_ context.Context, alias, agent, backend string) error {
	if f.cleanupErr != nil {
		return f.cleanupErr
	}
	f.accounts = append(f.accounts, alias)
	return nil
}
func accountEraseStack(t *testing.T) (*accountEraseRuntime, resource.Server, *sql.DB) {
	t.Helper()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db") + "?_pragma=foreign_keys(1)", MaxOpenConns: 1}).Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &accountEraseRuntime{fixture: &fixture{}, projects: []*api.Project{{Id: "p1", Name: "One", Workspace: "/one"}, {Id: "p2", Name: "Two", Workspace: "/two"}}}
	for i, item := range []struct{ id, account, project, state string }{{"first", "work", "p1", "idle"}, {"second", "work", "p2", "stopped"}, {"other", "other", "p1", "idle"}} {
		f.sessions = append(f.sessions, &api.Session{Id: item.id, ProjectId: item.project, Title: item.id, Agent: "claude", Account: item.account, AuthBackend: accounts.ProjectLocalOAuth, AuthBinding: accounts.BindingID(item.project, item.account, accounts.ProjectLocalOAuth), State: item.state, RunId: "run", CreateId: item.id, CreatedAt: int64(i + 1)})
	}
	f.fixture.p = f.projects[0]
	f.fixture.s = f.sessions[0]
	stack, err := Build(t.Context(), db, f, nil, Options{Notifications: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stack.Session().List(t.Context(), &resource.SessionListRequest{}); err != nil {
		t.Fatal(err)
	}
	return f, stack, db
}
func TestAccountEraseCascadesAcrossProjectsAndDoesNotResurrect(t *testing.T) {
	f, stack, db := accountEraseStack(t)
	ctx := t.Context()
	a, err := stack.Account().Get(ctx, resource.AccountGetRequest_builder{Ref: accountRef("work"), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	ref := resource.AccountRef_builder{Id: a.GetId()}.Build()
	out, err := stack.Account().Erase(ctx, ref)
	if err != nil || !out.GetErased() {
		t.Fatal(out, err)
	}
	if len(f.purged) != 2 || len(f.stopped) != 1 || f.stopped[0] != "first" || len(f.accounts) != 1 || f.accounts[0] != "work" {
		t.Fatal(f.purged, f.stopped, f.accounts)
	}
	for _, q := range []string{"SELECT count(*) FROM account WHERE id=?", "SELECT count(*) FROM session WHERE account_id=?", "SELECT count(*) FROM authbinding WHERE account_id=?", "SELECT count(*) FROM audit WHERE object_id=?"} {
		var count int
		if err := db.QueryRowContext(ctx, q, projectid.String(a.GetId())).Scan(&count); err != nil || count != 0 {
			t.Fatal(q, count, err)
		}
	}
	out, err = stack.Account().Erase(ctx, ref)
	if err != nil || out.GetErased() || len(f.accounts) != 1 {
		t.Fatal("retry", out, err, f.accounts)
	}
	rebuilt, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := rebuilt.Session().List(ctx, &resource.SessionListRequest{})
	if err != nil || len(page.GetItems()) != 1 || page.GetItems()[0].GetRuntimeId() != "other" {
		t.Fatal("session resurrection", page, err)
	}
	projects, err := rebuilt.Project().List(ctx, &resource.ProjectListRequest{})
	if err != nil || len(projects.GetItems()) != 2 {
		t.Fatal("projects removed", projects, err)
	}
	if _, err := rebuilt.Account().Get(ctx, resource.AccountGetRequest_builder{Ref: accountRef("work")}.Build()); status.Code(err) != codes.NotFound {
		t.Fatal("account resurrection", err)
	}
	if _, err := rebuilt.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: "work", Agent: "claude"}.Build()); err != nil {
		t.Fatal("erased alias not reusable", err)
	}
}
func TestAccountEraseFailureRemainsRetryable(t *testing.T) {
	for _, stage := range []string{"session", "credentials"} {
		t.Run(stage, func(t *testing.T) {
			f, stack, _ := accountEraseStack(t)
			if stage == "session" {
				f.failSession = "second"
			} else {
				f.cleanupErr = errors.New("credential cleanup failed")
			}
			if _, err := stack.Account().Erase(t.Context(), accountRef("work")); err == nil {
				t.Fatal("ignored cleanup failure")
			}
			if _, err := stack.Account().Get(t.Context(), resource.AccountGetRequest_builder{Ref: accountRef("work")}.Build()); err != nil {
				t.Fatal("failed cleanup hid account", err)
			}
			f.failSession = ""
			f.cleanupErr = nil
			if _, err := stack.Account().Erase(t.Context(), accountRef("work")); err != nil {
				t.Fatal("retry failed", err)
			}
			if len(f.purged) != 2 || len(f.accounts) != 1 {
				t.Fatal("duplicate purge", f.purged, f.accounts)
			}
		})
	}
}
func TestAccountEraseChecksAuthorityBeforeEffects(t *testing.T) {
	f, stack, _ := accountEraseStack(t)
	f.authorityErr = status.Error(codes.PermissionDenied, "manager required")
	if _, err := stack.Account().Erase(t.Context(), accountRef("work")); status.Code(err) != codes.PermissionDenied {
		t.Fatal(err)
	}
	if len(f.purged) != 0 || len(f.accounts) != 0 || len(f.stopped) != 0 {
		t.Fatal("unauthorized effects", f)
	}
	if _, err := stack.Account().Erase(t.Context(), &resource.AccountRef{}); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}

func TestAccountEraseDatabaseFailureKeepsAccountAndBindingsForRetry(t *testing.T) {
	_, stack, db := accountEraseStack(t)
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_account_delete BEFORE DELETE ON account WHEN OLD.alias='work' BEGIN SELECT RAISE(FAIL,'test cleanup failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := stack.Account().Erase(ctx, accountRef("work")); err == nil {
		t.Fatal("database failure ignored")
	}
	if _, err := stack.Account().Get(ctx, resource.AccountGetRequest_builder{Ref: accountRef("work")}.Build()); err != nil {
		t.Fatal("database failure hid account", err)
	}
	bindings, err := stack.AuthBinding().List(ctx, resource.AuthBindingListRequest_builder{Filters: []*resource.AuthBindingFilter{resource.AuthBindingFilter_builder{Account: accountRef("work")}.Build()}}.Build())
	if err != nil || len(bindings.GetItems()) != 2 {
		t.Fatal("database failure hid bindings", bindings, err)
	}
	if _, err := db.ExecContext(ctx, "DROP TRIGGER fail_account_delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := stack.Account().Erase(ctx, accountRef("work")); err != nil {
		t.Fatal("retry", err)
	}
}
