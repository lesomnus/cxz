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
	"google.golang.org/protobuf/proto"
)

type forkFixture struct {
	titleFixture
	request *api.SessionForkInput
}

func (f *forkFixture) ForkSession(_ context.Context, r *api.SessionForkInput) (*api.Session, error) {
	f.request = r
	if r.RunId != f.sessions[0].RunId {
		return nil, status.Error(codes.FailedPrecondition, "stale run_id")
	}
	child := proto.Clone(f.sessions[0]).(*api.Session)
	child.Id = "child"
	child.CreateId = r.ClientId
	child.RunId = "child-run"
	f.sessions = append(f.sessions, child)
	return child, nil
}

func TestForkResolvesSourceAndPublishesChildWithSameResourceEdges(t *testing.T) {
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "fork.db"), MaxOpenConns: 1}).Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &forkFixture{titleFixture: titleFixture{fixture: fixture{p: &api.Project{Id: "project", Workspace: "/workspace", Name: "Project"}}, sessions: []*api.Session{{Id: "parent", ProjectId: "project", Title: "Conversation", Agent: "codex", Model: "selected-model", Account: "work", AuthBackend: accounts.BrokeredAccessToken, AuthBinding: accounts.BindingID("project", "work", accounts.BrokeredAccessToken), CreateId: "parent-create", RunId: "parent-run", State: "idle", CreatedAt: time.Now().UnixMilli()}}}}
	stack, err := Build(t.Context(), db, f)
	if err != nil {
		t.Fatal(err)
	}
	source, err := stack.Session().Get(t.Context(), resource.SessionGetRequest_builder{Ref: sessionRef("parent"), Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build(), Account: resource.AccountSelect_builder{All: ptr(true)}.Build(), AuthBinding: resource.AuthBindingSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	child, err := stack.Session().Fork(t.Context(), resource.SessionControl_builder{Ref: resource.SessionRef_builder{Alias: ptr(source.GetAlias())}.Build(), RunId: ptr("parent-run"), ClientId: ptr("fork-key")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if f.request.SessionId != "parent" || f.request.ClientId != "fork-key" || child.GetRuntimeId() != "child" || child.GetAlias() == source.GetAlias() || child.GetModel() != source.GetModel() || child.GetName() != source.GetName() {
		t.Fatal("fork mapping/publishing failed", f.request, child)
	}
	child, err = stack.Session().Get(t.Context(), resource.SessionGetRequest_builder{Ref: sessionRef("child"), Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build(), Account: resource.AccountSelect_builder{All: ptr(true)}.Build(), AuthBinding: resource.AuthBindingSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
	if err != nil || child.GetProject().GetRuntimeId() != source.GetProject().GetRuntimeId() || child.GetAccount().GetAlias() != source.GetAccount().GetAlias() || child.GetAuthBinding().GetBindingId() != source.GetAuthBinding().GetBindingId() {
		t.Fatal("fork lost resource edges", child, err)
	}
}
