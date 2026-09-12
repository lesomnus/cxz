package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
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
	f.s.AuthBackend = accounts.ProjectLocalOAuth
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"personal", "work"} {
		a, err := stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: alias, Name: alias, Agent: "codex", AuthBackend: accounts.ProjectLocalOAuth}.Build())
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
	for _, backend := range []string{accounts.APIKey, "unknown"} {
		_, err = stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: "unsupported", Agent: "codex", AuthBackend: backend}.Build())
		if status.Code(err) != codes.InvalidArgument {
			t.Fatal("unsupported backend accepted", err)
		}
	}
	bindRequest := resource.AuthBindingAddRequest_builder{Account: accountRef("work"), Project: projectRef("project")}.Build()
	binding, err := stack.AuthBinding().Add(ctx, bindRequest)
	if err != nil {
		t.Fatal(err)
	}
	again, err := stack.AuthBinding().Add(ctx, bindRequest)
	if err != nil || binding.GetBindingId() != again.GetBindingId() {
		t.Fatal("binding retry not idempotent", err)
	}
	if binding.GetId()[9] != 10 || binding.GetScope() != "project" || binding.GetCredentialRef() != "accounts/work" || binding.GetAuthBackend() != accounts.ProjectLocalOAuth {
		t.Fatal("invalid binding", binding)
	}
	_, err = stack.AuthBinding().Patch(ctx, resource.AuthBindingPatchRequest_builder{Ref: bindingRef(binding.GetBindingId())}.Build())
	if status.Code(err) != codes.PermissionDenied {
		t.Fatal("binding mutation accepted", err)
	}
	_, err = stack.AuthBinding().Add(ctx, resource.AuthBindingAddRequest_builder{Account: accountRef("work"), Project: projectRef("project"), CredentialRef: "/arbitrary/path"}.Build())
	if status.Code(err) != codes.PermissionDenied {
		t.Fatal("caller-selected credential path accepted", err)
	}
	f.p = &api.Project{Id: "other-project", Workspace: "/workspaces/other", Name: "other"}
	if _, err = stack.Project().Add(ctx, resource.ProjectAddRequest_builder{Workspace: f.p.Workspace}.Build()); err != nil {
		t.Fatal(err)
	}
	other, err := stack.AuthBinding().Add(ctx, resource.AuthBindingAddRequest_builder{Account: accountRef("work"), Project: projectRef(f.p.Id)}.Build())
	if err != nil || other.GetBindingId() == binding.GetBindingId() {
		t.Fatal("cross-project auth sharing", err)
	}
	_, err = stack.Session().Add(ctx, resource.SessionAddRequest_builder{Project: projectRef("project"), Account: accountRef("work"), AuthBinding: bindingRef(other.GetBindingId()), Agent: "codex", ClientId: "wrong-binding"}.Build())
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal("cross-project session binding accepted", err)
	}
	_, err = stack.Session().Add(ctx, resource.SessionAddRequest_builder{Project: projectRef("project"), ClientId: "wrong-vendor", Agent: "claude", Account: accountRef("work"), AuthBinding: bindingRef(f.s.AuthBinding)}.Build())
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal("wrong vendor accepted", err)
	}
	_, err = stack.Session().Add(ctx, resource.SessionAddRequest_builder{Project: projectRef("project"), ClientId: "unknown", Agent: "codex", Account: accountRef("unknown"), AuthBinding: bindingRef(f.s.AuthBinding)}.Build())
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
