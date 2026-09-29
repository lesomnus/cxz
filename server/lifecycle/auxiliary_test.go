package lifecycle

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"path/filepath"
	"testing"
)

type auxiliaryFixture struct {
	fixture
	got   auxiliary.Request
	calls int
}

func (f *auxiliaryFixture) Docker(_ context.Context, r *api.DockerInput) (*api.Receipt, error) {
	f.calls++
	_ = json.Unmarshal(r.Spec, &f.got)
	return &api.Receipt{Status: `{}`}, nil
}
func TestAuxiliaryUsesRegisteredAccountNotClientProvider(t *testing.T) {
	ctx := t.Context()
	db, _, e := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db") + "?_pragma=foreign_keys(1)", MaxOpenConns: 1}).Open(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	f := &auxiliaryFixture{}
	stack, e := Build(ctx, db, f)
	if e != nil {
		t.Fatal(e)
	}
	_, e = stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: "work", Agent: "codex", AuthBackend: accounts.BrokeredAccessToken}.Build())
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(auxiliary.Request{Action: "models", Profile: auxiliary.Profile{Account: "work", Agent: "claude", Backend: accounts.ProjectLocalOAuth}})
	_, e = stack.Project().Docker(ctx, resource.DockerRequest_builder{Action: ptr("auxiliary"), Spec: b}.Build())
	if e != nil {
		t.Fatal(e)
	}
	if f.got.Profile.Agent != "codex" || f.got.Profile.Backend != accounts.BrokeredAccessToken {
		t.Fatal("trusted client authentication fields")
	}
	b, _ = json.Marshal(auxiliary.Request{Action: "models", Profile: auxiliary.Profile{Account: "missing"}})
	_, e = stack.Project().Docker(ctx, resource.DockerRequest_builder{Action: ptr("auxiliary"), Spec: b}.Build())
	if e == nil || f.calls != 1 {
		t.Fatal("missing account reached runner")
	}
}
