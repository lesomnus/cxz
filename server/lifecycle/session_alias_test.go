package lifecycle

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/sessionalias"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSessionAliasPersistenceAndMigration(t *testing.T) {
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &fixture{p: &api.Project{Id: "project", Workspace: "/workspaces/test", Name: "test"}, s: &api.Session{Id: "first", ProjectId: "project", Agent: "codex", Account: "work", CreateId: "create", CreatedAt: time.Now().UnixMilli()}}
	f.s.AuthBackend = accounts.ProjectLocalOAuth
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	get := func(id string) *resource.Session {
		t.Helper()
		s, e := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef(id), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	first := get("first")
	if !sessionalias.Valid(first.GetAlias()) {
		t.Fatal(first)
	}
	// Recreate the pre-alias schema with an existing row, then run real migration.
	if _, err = db.ExecContext(ctx, "DROP INDEX session_alias"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "ALTER TABLE session DROP COLUMN alias"); err != nil {
		t.Fatal(err)
	}
	stack, err = Build(ctx, db, f)
	if err != nil {
		t.Fatal("upgrade", err)
	}
	first = get("first")
	if !sessionalias.Valid(first.GetAlias()) {
		t.Fatal("backfill", first)
	}
	_, err = stack.Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef("first"), Alias: ptr("orchard")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	stack, err = Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	if get("first").GetAlias() != "orchard" {
		t.Fatal("rename lost on restart/snapshot")
	}
	f.s.Id = "second"
	f.s.CreateId = "second-create"
	second := get("second")
	if second.GetAlias() == "orchard" {
		t.Fatal("duplicate allocation")
	}
	_, err = stack.Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef("second"), Alias: ptr("orchard")}.Build())
	if status.Code(err) != codes.AlreadyExists {
		t.Fatal("duplicate rename", err)
	}
	_, err = stack.Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef("second"), Alias: ptr("ab")}.Build())
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal("invalid rename", err)
	}
	_, err = stack.Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef("second"), Alias: ptr("cedar"), Listed: ptr(false)}.Build())
	if status.Code(err) != codes.PermissionDenied {
		t.Fatal("metadata bypass", err)
	}
	s, err := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: resource.SessionRef_builder{Alias: ptr("orchard")}.Build()}.Build())
	if err != nil || s.GetRuntimeId() != "first" {
		t.Fatal("alias resolution", err)
	}
}
