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
	reply string
}

func (f *auxiliaryFixture) Docker(_ context.Context, r *api.DockerInput) (*api.Receipt, error) {
	f.calls++
	_ = json.Unmarshal(r.Spec, &f.got)
	if f.reply != "" {
		return &api.Receipt{Status: f.reply}, nil
	}
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

func TestAuxiliaryTitleUpdatesResourceWithoutChangingIdentity(t *testing.T) {
	ctx := t.Context()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &auxiliaryFixture{fixture: fixture{p: &api.Project{Id: "project", Workspace: "/work", Name: "test"}, s: &api.Session{Id: "session", ProjectId: "project", Agent: "codex", Title: "Old title", LastSeq: 7, Account: "work", AuthBackend: accounts.ProjectLocalOAuth}}}
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	get := func() *resource.Session {
		v, e := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef("session"), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	before := get()
	f.reply = `{"title":{"text":"New title","status":"completed"}}`
	b, _ := json.Marshal(auxiliary.Request{Action: "status", Session: "session"})
	if _, err := stack.Project().Docker(ctx, resource.DockerRequest_builder{Action: ptr("auxiliary"), Spec: b}.Build()); err != nil {
		t.Fatal(err)
	}
	after := get()
	if after.GetName() != "New title" || string(after.GetId()) != string(before.GetId()) || after.GetAlias() != before.GetAlias() {
		t.Fatal("title or identity", after)
	}
	// Inventory reconciliation must propagate a title even with unchanged status.
	f.s.Title = "Final title"
	layer, _ := resource.Find[Layer](stack)
	if err := layer.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if get().GetName() != "Final title" {
		t.Fatal("same-sequence title change dropped")
	}
}
