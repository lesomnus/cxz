package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type sessionStartFixture struct {
	fixture
	creates   int
	createErr error
	erases    int
}

func (f *sessionStartFixture) ResourceSnapshot(context.Context) (*api.ProjectList, *api.SessionList, error) {
	list := &api.SessionList{}
	if f.s != nil {
		list.Sessions = []*api.Session{f.s}
	}
	return &api.ProjectList{Projects: []*api.Project{f.p}}, list, nil
}

func (f *sessionStartFixture) Create(_ context.Context, r *api.CreateRequest) (*api.Session, error) {
	f.creates++
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.s = &api.Session{Id: core.SessionID(r.ClientId), CreateId: r.ClientId, Title: r.Title, Agent: r.Agent, Model: r.Model, Account: r.Account, AuthBackend: r.AuthBackend, AuthBinding: r.AuthBinding, State: "idle", RunId: "first-run"}
	return f.s, nil
}

func (f *sessionStartFixture) EraseSession(context.Context, *api.SessionEraseInput) (*api.SessionEraseReply, error) {
	f.erases++
	return &api.SessionEraseReply{}, nil
}

func TestSessionAddPersistsWithoutRuntimeAndStartRetriesTheSameRecord(t *testing.T) {
	ctx := t.Context()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &sessionStartFixture{fixture: fixture{p: &api.Project{Id: "project", Workspace: "/work", Name: "Work", State: "stopped"}}}
	stack, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stack.Project().Add(ctx, resource.ProjectAddRequest_builder{Workspace: "/work"}.Build()); err != nil {
		t.Fatal(err)
	}
	if _, err = stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: "work", Agent: "codex", AuthBackend: accounts.BrokeredAccessToken}.Build()); err != nil {
		t.Fatal(err)
	}
	b, err := stack.AuthBinding().Add(ctx, resource.AuthBindingAddRequest_builder{Project: projectRef("project"), Account: accountRef("work")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	request := resource.SessionAddRequest_builder{Project: projectRef("project"), Account: accountRef("work"), AuthBinding: bindingRef(b.GetBindingId()), ClientId: "add-with-stopped-project", Name: "New session", Desc: "Retained description"}.Build()
	added, err := stack.Session().Add(ctx, request)
	if err != nil || f.creates != 0 || added.GetAlias() == "" || added.GetStatus().GetState() != "created" {
		t.Fatal(added, f.creates, err)
	}
	again, err := stack.Session().Add(ctx, request)
	if err != nil || !bytes.Equal(added.GetId(), again.GetId()) {
		t.Fatal("Add retry", again, err)
	}
	request.SetName("Different request")
	if _, err = stack.Session().Add(ctx, request); status.Code(err) != codes.AlreadyExists {
		t.Fatal("reused creation key", err)
	}
	ref := resource.SessionRef_builder{Id: added.GetId()}.Build()
	if _, err = stack.Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: ref, Name: ptr("Edited before starting")}.Build()); err != nil {
		t.Fatal("edit before start", err)
	}
	listed, err := stack.Session().List(ctx, &resource.SessionListRequest{})
	if err != nil || len(listed.GetItems()) != 1 || f.creates != 0 {
		t.Fatal("List unstarted", listed, err)
	}
	if page, err := stack.Session().History(ctx, resource.SessionEventsRequest_builder{Ref: ref}.Build()); err != nil || len(page.GetEvents()) != 0 {
		t.Fatal("unstarted history called runtime", page, err)
	}
	if page, err := stack.Session().Transcript(ctx, resource.SessionTranscriptRequest_builder{Ref: ref}.Build()); err != nil || len(page.GetEvents()) != 0 {
		t.Fatal("unstarted transcript called runtime", page, err)
	}
	layer, _ := resource.Find[Layer](stack)
	reader := SessionServer{Layer: layer, SessionServiceServer: layer.Next().Session()}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := reader.awaitFirstRun(canceled, added, func() error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal("unstarted stream ignored cancellation", err)
	}
	control := resource.SessionControl_builder{Ref: ref, ClientId: ptr("start")}.Build()
	f.createErr = errors.New("project is stopped")
	if _, err = stack.Session().Start(ctx, control); err == nil {
		t.Fatal("expected start failure")
	}
	retained, err := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil || retained.GetStatus().GetState() != "failed" || retained.GetAlias() != added.GetAlias() {
		t.Fatal("failed start lost identity", retained, err)
	}
	f.createErr = nil
	started, err := stack.Session().Start(ctx, control)
	if err != nil || !bytes.Equal(started.GetId(), added.GetId()) || started.GetStatus().GetState() != "idle" || f.creates != 2 || f.s.Title != "Edited before starting" || started.GetDesc() != "Retained description" {
		t.Fatal("start retry", started, f.creates, err)
	}
	if _, err = stack.Session().Start(ctx, control); err != nil || f.creates != 2 {
		t.Fatal("duplicate first start", err)
	}
	control.SetRunId("stale")
	if _, err = stack.Session().Start(ctx, control); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("stale run", err)
	}
	request.SetClientId("never-started")
	unstarted, err := stack.Session().Add(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stack.Session().Erase(ctx, resource.SessionRef_builder{Id: unstarted.GetId()}.Build()); err != nil || f.erases != 0 {
		t.Fatal("erase unstarted called runtime", f.erases, err)
	}
}
