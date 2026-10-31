package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/sessionpurge"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type purgeFixture struct {
	deletionFixture
	purged        []sessionpurge.Request
	eraseErr      error
	erasedSession bool
}

func (f *purgeFixture) EraseSession(_ context.Context, r *api.SessionEraseInput) (*api.SessionEraseReply, error) {
	if f.eraseErr != nil {
		return nil, f.eraseErr
	}
	f.erasedSession = true
	f.purged = append(f.purged, sessionpurge.Request{Session: r.SessionId, DryRun: r.DryRun})
	return &api.SessionEraseReply{
		SessionId: r.SessionId, DryRun: r.DryRun,
		Targets: []*api.SessionEraseTarget{{Kind: "journal", Path: "/state/sessions/" + r.SessionId, Files: 3, Bytes: 4096}},
	}, nil
}

func purgeStack(t *testing.T) (*purgeFixture, resource.Server, *sql.DB, func()) {
	t.Helper()
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f := &purgeFixture{deletionFixture: deletionFixture{fixture: fixture{
		p: &api.Project{Id: "project", Workspace: "/work", Name: "work"},
		s: &api.Session{Id: "session", ProjectId: "project", Workspace: "/work", Title: "secrets in the title", Agent: "claude", Account: "work", AuthBackend: accounts.ProjectLocalOAuth, State: "idle", RunId: "run", CreateId: "create"},
	}}}
	f.s.AuthBinding = accounts.BindingID("project", "work", accounts.ProjectLocalOAuth)
	stack, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stack.Session().List(ctx, &resource.SessionListRequest{}); err != nil {
		t.Fatal(err)
	}
	return f, stack, db, func() { db.Close() }
}

func (f *purgeFixture) ResourceSnapshot(ctx context.Context) (*api.ProjectList, *api.SessionList, error) {
	if f.erasedSession {
		return &api.ProjectList{Projects: []*api.Project{f.p}}, &api.SessionList{}, nil
	}
	return f.deletionFixture.ResourceSnapshot(ctx)
}
func TestEraseDeletesByEverySessionHandle(t *testing.T) {
	for _, kind := range []string{"runtime", "alias", "uuid"} {
		t.Run(kind, func(t *testing.T) {
			f, stack, db, done := purgeStack(t)
			defer done()
			ctx := t.Context()
			v, err := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef("session"), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
			if err != nil {
				t.Fatal(err)
			}
			ref := sessionRef("session")
			if kind == "alias" {
				ref = resource.SessionRef_builder{Alias: ptr(v.GetAlias())}.Build()
			}
			if kind == "uuid" {
				ref = resource.SessionRef_builder{Id: v.GetId()}.Build()
			}
			out, err := stack.Session().Erase(ctx, ref)
			if err != nil || !out.GetErased() || f.stops != 1 || len(f.purged) != 1 || f.purged[0].Session != "session" {
				t.Fatal(out, err, f.stops, f.purged)
			}
			var count int
			if err = db.QueryRowContext(ctx, "SELECT count(*) FROM session").Scan(&count); err != nil || count != 0 {
				t.Fatal("session record retained", count, err)
			}
			if _, err = stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: ref}.Build()); status.Code(err) != codes.NotFound {
				t.Fatal("erased session readable", err)
			}
			out, err = stack.Session().Erase(ctx, ref)
			if err != nil || out.GetErased() || f.stops != 1 || len(f.purged) != 1 {
				t.Fatal("erase not idempotent", out, err)
			}
			rebuilt, err := Build(ctx, db, f, nil)
			if err != nil {
				t.Fatal(err)
			}
			page, err := rebuilt.Session().List(ctx, &resource.SessionListRequest{})
			if err != nil || len(page.GetItems()) != 0 {
				t.Fatal("erased session resurrected", page, err)
			}
			projects, err := rebuilt.Project().List(ctx, &resource.ProjectListRequest{})
			if err != nil || len(projects.GetItems()) != 1 {
				t.Fatal("erase affected owning project", err)
			}
		})
	}
}
func TestEraseFailureLeavesRecordAndAliasForRetry(t *testing.T) {
	f, stack, _, done := purgeStack(t)
	defer done()
	ctx := t.Context()
	f.eraseErr = errors.New("cleanup failed")
	if _, err := stack.Session().Erase(ctx, sessionRef("session")); err == nil {
		t.Fatal("ignored cleanup failure")
	}
	page, err := stack.Session().List(ctx, &resource.SessionListRequest{})
	if err != nil || len(page.GetItems()) != 1 || page.GetItems()[0].GetAlias() == "" || page.GetItems()[0].GetName() != "secrets in the title" {
		t.Fatal("failed erase hid or changed resource", page, err)
	}
	f.eraseErr = nil
	if _, err = stack.Session().Erase(ctx, sessionRef("session")); err != nil {
		t.Fatal("retry failed", err)
	}
}
func TestSessionListingMigrationRetainsPreviouslyHiddenRecords(t *testing.T) {
	f, stack, db, done := purgeStack(t)
	defer done()
	ctx := t.Context()
	before, err := stack.Session().List(ctx, &resource.SessionListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "ALTER TABLE session ADD COLUMN listed BOOL NOT NULL DEFAULT true"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "UPDATE session SET listed=false,alias=NULL"); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	after, err := rebuilt.Session().List(ctx, &resource.SessionListRequest{})
	if err != nil || len(after.GetItems()) != 1 || string(after.GetItems()[0].GetId()) != string(before.GetItems()[0].GetId()) || after.GetItems()[0].GetName() != before.GetItems()[0].GetName() || after.GetItems()[0].GetAlias() == "" {
		t.Fatal("migration lost hidden session or its identity", after, err)
	}
	if _, err = db.ExecContext(ctx, "SELECT listed FROM session"); err == nil {
		t.Fatal("legacy listing column retained")
	}
}
func TestSessionContractHasNoPurgeRestoreOrListed(t *testing.T) {
	service := resource.File_cxz_session_svc_g_proto.Services().ByName("SessionService")
	if service.Methods().ByName("Erase") == nil || service.Methods().ByName("Purge") != nil || service.Methods().ByName("Restore") != nil {
		t.Fatal("obsolete lifecycle RPC contract")
	}
	if (&resource.Session{}).ProtoReflect().Descriptor().Fields().ByName("listed") != nil {
		t.Fatal("server listing field retained")
	}
}
