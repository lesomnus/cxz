package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type sharedAuxFixture struct {
	fixture
	saved       []*api.AuxProfile
	configCalls int
}

func (f *sharedAuxFixture) AuxSetConfig(_ context.Context, in *api.AuxSetConfigInput) (*api.AuxConfigReply, error) {
	f.configCalls++
	f.saved = in.Profiles
	return &api.AuxConfigReply{Profiles: f.saved}, nil
}

func TestAuxiliarySharedAccountResolvesEveryKindAndRejectsMixedSaves(t *testing.T) {
	ctx := t.Context()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &sharedAuxFixture{}
	stack, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"work", "other"} {
		if _, err = stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: alias, Agent: "codex", AuthBackend: accounts.BrokeredAccessToken}.Build()); err != nil {
			t.Fatal(err)
		}
	}
	layer, _ := resource.Find[Layer](stack)
	profiles := []*resource.AgentProfile{}
	for _, kind := range []resource.AuxKind{resource.AuxKind_AUX_KIND_SUMMARY, resource.AuxKind_AUX_KIND_SUGGESTION, resource.AuxKind_AUX_KIND_TITLE} {
		profiles = append(profiles, resource.AgentProfile_builder{Kind: &kind, Agent: ptr("forged"), Backend: ptr("forged"), Model: ptr("test")}.Build())
	}
	out, err := layer.Installation().SaveAgents(ctx, resource.SaveAgentsRequest_builder{Account: ptr("work"), Profiles: profiles}.Build())
	if err != nil || out.GetAccount() != "work" || len(f.saved) != 3 {
		t.Fatal(out, err)
	}
	for _, p := range f.saved {
		if p.Account != "work" || p.Agent != "codex" || p.Backend != accounts.BrokeredAccessToken {
			t.Fatal("wrong shared identity", p)
		}
	}
	profiles[1].SetAccount("other")
	if _, err = layer.Installation().SaveAgents(ctx, resource.SaveAgentsRequest_builder{Account: ptr("work"), Profiles: profiles}.Build()); status.Code(err) != codes.InvalidArgument || f.configCalls != 1 {
		t.Fatal("mixed accounts reached runtime", err, f.configCalls)
	}
	profiles[0].SetAccount("work")
	if _, err = layer.Installation().SaveAgents(ctx, resource.SaveAgentsRequest_builder{Profiles: profiles}.Build()); status.Code(err) != codes.InvalidArgument || f.configCalls != 1 {
		t.Fatal("legacy mixed accounts reached runtime", err, f.configCalls)
	}
}
