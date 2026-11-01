package lifecycle

import (
	"bytes"
	"context"
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

type accountEditFixture struct {
	*accountStatusFixture
	configured string
	created    *api.CreateRequest
}

func (f *accountEditFixture) Create(_ context.Context, in *api.CreateRequest) (*api.Session, error) {
	f.created = in
	return &api.Session{Id: "created-" + in.ClientId, CreateId: in.ClientId, Agent: in.Agent, Account: in.Account, AuthBackend: in.AuthBackend, AuthBinding: in.AuthBinding, State: "idle", CreatedAt: 1}, nil
}

func (f *accountEditFixture) AuxConfig(context.Context, *api.Empty) (*api.AuxConfigReply, error) {
	return &api.AuxConfigReply{Profiles: []*api.AuxProfile{{Kind: "summary", Account: f.configured, Agent: "codex", Backend: accounts.BrokeredAccessToken, Enabled: true}}}, nil
}
func (f *accountEditFixture) AuxSetConfig(_ context.Context, in *api.AuxSetConfigInput) (*api.AuxConfigReply, error) {
	f.configured = in.Profiles[0].Account
	return f.AuxConfig(context.Background(), &api.Empty{})
}

func TestAccountMetadataPreservesRuntimeIdentity(t *testing.T) {
	ctx := t.Context()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db") + "?_pragma=foreign_keys(1)", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &accountEditFixture{accountStatusFixture: &accountStatusFixture{fixture: &fixture{p: &api.Project{Id: "project", Workspace: "/work", Name: "Work"}, s: &api.Session{Id: "session", ProjectId: "project", Account: "work", Agent: "codex", AuthBackend: accounts.BrokeredAccessToken, AuthBinding: accounts.BindingID("project", "work", accounts.BrokeredAccessToken)}}}, configured: "work"}
	stack, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	original, err := stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: "work", Name: "Original", Agent: "codex"}.Build())
	if err != nil {
		t.Fatal(err)
	}
	// An old installation has no private mapping; startup backfills it before edits.
	if _, err = db.ExecContext(ctx, "DELETE FROM account_runtime_identity"); err != nil {
		t.Fatal(err)
	}
	stack, err = Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := stack.Account().Patch(ctx, resource.AccountPatchRequest_builder{Ref: accountRef("work"), Alias: ptr("renamed"), Name: ptr("New display name"), DateUpdated: original.GetDateUpdated()}.Build())
	if err != nil || !bytes.Equal(original.GetId(), renamed.GetId()) || renamed.GetName() != "New display name" {
		t.Fatal("rename", renamed, err)
	}
	if _, err = stack.Account().Status(ctx, accountRef("renamed")); err != nil || f.profile != "work" {
		t.Fatal("login identity changed", f.profile, err)
	}
	session, err := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef("session"), Select: resource.SessionSelect_builder{All: ptr(true), Account: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
	if err != nil || !bytes.Equal(session.GetAccount().GetId(), original.GetId()) || session.GetAccount().GetAlias() != "renamed" {
		t.Fatal("session account changed", session, err)
	}
	binding, err := stack.AuthBinding().Add(ctx, resource.AuthBindingAddRequest_builder{Account: accountRef("renamed"), Project: projectRef("project")}.Build())
	if err != nil || binding.GetBindingId() != accounts.BindingID(projectid.String(session.GetProject().GetId()), "work", accounts.BrokeredAccessToken) || binding.GetCredentialRef() != "central/accounts/work" {
		t.Fatal("binding changed", binding, err)
	}
	created, err := stack.Session().Add(ctx, resource.SessionAddRequest_builder{Project: projectRef("project"), Account: accountRef("renamed"), AuthBinding: bindingRef(binding.GetBindingId()), ClientId: "after-rename"}.Build())
	if err != nil || f.created.Account != "work" || !bytes.Equal(created.GetAccount().GetId(), original.GetId()) {
		t.Fatal("new session changed login profile", created, err)
	}
	layer, ok := resource.Find[Layer](stack)
	if !ok {
		t.Fatal("missing lifecycle layer")
	}
	profiles, err := layer.Installation().Agents(ctx, &resource.AgentsRequest{})
	if err != nil || profiles.GetProfiles()[0].GetAccount() != "renamed" {
		t.Fatal("configured alias", profiles, err)
	}
	_, err = layer.Installation().SaveAgents(ctx, resource.SaveAgentsRequest_builder{Profiles: profiles.GetProfiles()}.Build())
	if err != nil || f.configured != "work" {
		t.Fatal("configured identity changed", f.configured, err)
	}
	reused, err := stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: "work", Agent: "codex"}.Build())
	if err != nil || bytes.Equal(reused.GetId(), original.GetId()) {
		t.Fatal("reused alias shared identity", reused, err)
	}
	if _, err = stack.Account().Status(ctx, accountRef("work")); err != nil || f.profile == "work" {
		t.Fatal("reused alias shared credentials", f.profile, err)
	}
	for _, request := range []*resource.AccountPatchRequest{
		resource.AccountPatchRequest_builder{Ref: accountRef("renamed"), Name: ptr("Stale"), DateUpdated: original.GetDateUpdated()}.Build(),
		resource.AccountPatchRequest_builder{Ref: accountRef("renamed"), Alias: ptr("work"), DateUpdated: renamed.GetDateUpdated()}.Build(),
		resource.AccountPatchRequest_builder{Ref: accountRef("renamed"), Alias: ptr("Bad_Alias"), DateUpdated: renamed.GetDateUpdated()}.Build(),
		resource.AccountPatchRequest_builder{Ref: accountRef("renamed"), Name: ptr(" "), DateUpdated: renamed.GetDateUpdated()}.Build(),
	} {
		if _, err = stack.Account().Patch(ctx, request); err == nil {
			t.Fatal("invalid/conflicting edit succeeded", request)
		}
	}
	if _, err = stack.Account().Apply(ctx, &resource.AccountApplyRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatal("general writes opened", err)
	}
	stack, err = Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stack.Account().Status(ctx, accountRef("renamed")); err != nil || f.profile != "work" {
		t.Fatal("restart changed profile", f.profile, err)
	}
}
