package lifecycle

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type switchFixture struct {
	titleFixture
	request *api.SessionAccountInput
}

func (f *switchFixture) SwitchSessionAccount(_ context.Context, r *api.SessionAccountInput) (*api.Session, error) {
	f.request = r
	for _, v := range f.sessions {
		if v.Id == r.SessionId {
			if v.State != "stopped" {
				return nil, status.Error(codes.FailedPrecondition, "stop the session before switching its account")
			}
			v.Account, v.AuthBackend, v.AuthBinding = r.Account, r.AuthBackend, r.AuthBinding
			return v, nil
		}
	}
	return nil, status.Error(codes.NotFound, "session not found")
}

func (f *switchFixture) Get(_ context.Context, r *api.SessionRef) (*api.Session, error) {
	for _, v := range f.sessions {
		if v.Id == r.Id {
			return v, nil
		}
	}
	return nil, status.Error(codes.NotFound, "session not found")
}

func TestSwitchAccountRebindsResourceEdges(t *testing.T) {
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "switch.db"), MaxOpenConns: 1}).Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	backend := accounts.BrokeredAccessToken
	session := func(id, account, state string) *api.Session {
		return &api.Session{Id: id, ProjectId: "project", Title: id, Agent: "codex", Account: account, AuthBackend: backend, AuthBinding: accounts.BindingID("project", account, backend), CreateId: id + "-create", State: state, RunId: id + "-run", CreatedAt: time.Now().UnixMilli()}
	}
	f := &switchFixture{titleFixture: titleFixture{fixture: fixture{p: &api.Project{Id: "project", Workspace: "/workspace", Name: "Project"}}, sessions: []*api.Session{
		session("target", "work", "idle"),
		// Registers the second account the way any runtime session would.
		session("other", "personal", "stopped"),
	}}}
	stack, err := Build(t.Context(), db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	read := func() *resource.Session {
		v, err := stack.Session().Get(t.Context(), resource.SessionGetRequest_builder{Ref: sessionRef("target"), Select: resource.SessionSelect_builder{All: ptr(true), Account: resource.AccountSelect_builder{All: ptr(true)}.Build(), AuthBinding: resource.AuthBindingSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	before := read()
	request := resource.SessionSwitchAccountRequest_builder{Ref: resource.SessionRef_builder{Alias: ptr(before.GetAlias())}.Build(), Account: resource.AccountRef_builder{Alias: ptr("personal")}.Build()}.Build()
	if _, err = stack.Session().SwitchAccount(t.Context(), request); status.Code(err) != codes.FailedPrecondition || f.request != nil {
		t.Fatalf("switched a running session: %v", err)
	}
	// SwitchAccount reads the runtime state before deciding.
	f.sessions[0].State = "stopped"
	out, err := stack.Session().SwitchAccount(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if f.request == nil || f.request.SessionId != "target" || f.request.Account != "personal" || f.request.AuthBackend != backend {
		t.Fatalf("runtime request: %v", f.request)
	}
	after := read()
	if after.GetAccount().GetAlias() != "personal" || out.GetAccount().GetAlias() != "personal" {
		t.Fatalf("account edge not switched: %v", after.GetAccount())
	}
	if after.GetAuthBinding().GetBindingId() != f.request.AuthBinding {
		t.Fatalf("binding edge not switched: %v", after.GetAuthBinding())
	}
	if !after.GetDateUpdated().AsTime().After(before.GetDateUpdated().AsTime()) {
		t.Fatal("switch did not publish an update")
	}
	// The same account again is a no-op, not a second runtime call.
	f.request = nil
	if _, err = stack.Session().SwitchAccount(t.Context(), request); err != nil || f.request != nil {
		t.Fatalf("repeat switch: %v %v", err, f.request)
	}
	// Listing by account follows the switched edge.
	page, err := stack.Session().List(t.Context(), resource.SessionListRequest_builder{Filters: []*resource.SessionFilter{resource.SessionFilter_builder{Account: resource.AccountRef_builder{Alias: ptr("work")}.Build()}.Build()}}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetItems()) != 0 {
		t.Fatalf("old account still lists the session: %v", page.GetItems())
	}
}
