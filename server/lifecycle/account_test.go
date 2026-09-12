package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"path/filepath"
	"testing"
	"time"
)

func TestAccountResources(t *testing.T) {
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db") + "?_pragma=foreign_keys(1)", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &fixture{p: &api.Project{Id: "project", Workspace: "/workspaces/test", Name: "test"}, s: &api.Session{Id: "session", ProjectId: "project", Agent: "codex", Account: "work", CreateId: "create", CreatedAt: time.Now().UnixMilli()}}
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"personal", "work"} {
		a, err := stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: alias, Name: alias, Agent: "codex"}.Build())
		if err != nil {
			t.Fatal(err)
		}
		if a.GetAgent() != "codex" || a.GetId()[9] != 9 {
			t.Fatal("invalid account", a)
		}
	}
	_, err = stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: "work", Agent: "claude"}.Build())
	if status.Code(err) != codes.AlreadyExists {
		t.Fatal("duplicate account", err)
	}
	_, err = stack.Account().Patch(ctx, resource.AccountPatchRequest_builder{Ref: accountRef("work"), Alias: ptr("changed")}.Build())
	if status.Code(err) != codes.PermissionDenied {
		t.Fatal("account identity mutable", err)
	}
	_, err = stack.Account().Erase(ctx, accountRef("work"))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatal("account deletion accepted", err)
	}
	s, err := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef("session"), Select: resource.SessionSelect_builder{All: ptr(true), Account: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if s.GetAccount().GetAlias() != "work" {
		t.Fatal("account edge lost", s)
	}
	_, err = stack.Session().Add(ctx, resource.SessionAddRequest_builder{Project: projectRef("project"), ClientId: "wrong-vendor", Agent: "claude", Account: accountRef("work")}.Build())
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal("wrong vendor accepted", err)
	}
	_, err = stack.Session().Add(ctx, resource.SessionAddRequest_builder{Project: projectRef("project"), ClientId: "unknown", Agent: "codex", Account: accountRef("unknown")}.Build())
	if status.Code(err) != codes.NotFound {
		t.Fatal("unknown account accepted", err)
	}
	rebuilt, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	a, err := rebuilt.Account().Get(ctx, resource.AccountGetRequest_builder{Ref: accountRef("work"), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil || a.GetAgent() != "codex" {
		t.Fatal("account persistence", err)
	}
}
