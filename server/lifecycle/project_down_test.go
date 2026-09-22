package lifecycle

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/protobuf/proto"
)

type retainedProjectFixture struct{ deletionFixture }

func (f *retainedProjectFixture) History(_ context.Context, r *api.WatchRequest) (*api.EventBatch, error) {
	return &api.EventBatch{Events: []*api.Event{
		{SessionId: r.SessionId, Seq: 1, Kind: "input", Text: "Remember this conversation"},
		{SessionId: r.SessionId, Seq: 2, Kind: "text", Text: "An earlier response"},
	}}, nil
}

func TestProjectDownUpRetainsSessions(t *testing.T) {
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &retainedProjectFixture{deletionFixture: deletionFixture{fixture: fixture{
		p: &api.Project{Id: "project", Workspace: "/work", Name: "work", State: "running"},
		s: &api.Session{Id: "session", ProjectId: "project", Workspace: "/work", Title: "Existing conversation", Agent: "claude", Account: "work", AuthBackend: accounts.ProjectLocalOAuth, State: "idle", RunId: "run", CreateId: "create", VendorId: "vendor-conversation", LastSeq: 2},
	}}}
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	getProject := func() *resource.Project {
		t.Helper()
		p, err := stack.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: projectRef("project"), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	getSession := func() *resource.Session {
		t.Helper()
		s, err := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef("session"), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	p, s := getProject(), getSession()
	historyRequest := resource.SessionEventsRequest_builder{Ref: resource.SessionRef_builder{Alias: ptr(s.GetAlias())}.Build()}.Build()
	history, err := stack.Session().History(ctx, historyRequest)
	if err != nil || len(history.GetEvents()) != 2 {
		t.Fatal(history, err)
	}
	if _, err := stack.Project().Down(ctx, resource.ProjectControl_builder{Ref: projectRef("project"), ClientId: ptr("down")}.Build()); err != nil || f.downs != 1 {
		t.Fatal(err, f.downs)
	}
	for _, phase := range []string{"down", "up"} {
		if phase == "up" {
			if _, err := stack.Project().Up(ctx, resource.ProjectUpRequest_builder{Ref: projectRef("project"), ClientId: ptr("up")}.Build()); err != nil {
				t.Fatal(err)
			}
			if f.up == nil || !f.up.PrepareOnly || f.up.NewSession || f.resume != nil {
				t.Fatal("up must prepare the existing project without creating or resuming a session", f.up, f.resume)
			}
		}
		// Rebuild the resource stack to verify persistence and fresh runtime snapshots.
		stack, err = Build(ctx, db, f)
		if err != nil {
			t.Fatal(err)
		}
		projects, err := stack.Project().List(ctx, &resource.ProjectListRequest{})
		if err != nil || len(projects.GetItems()) != 1 {
			t.Fatal(phase, "project disappeared", projects, err)
		}
		sessions, err := stack.Session().List(ctx, &resource.SessionListRequest{})
		if err != nil || len(sessions.GetItems()) != 1 {
			t.Fatal(phase, "session disappeared", sessions, err)
		}
		gotP, gotS := getProject(), getSession()
		if !bytes.Equal(gotP.GetId(), p.GetId()) || gotP.GetAlias() != p.GetAlias() || gotP.GetName() != p.GetName() || !gotP.GetListed() {
			t.Fatal(phase, "project identity changed", gotP)
		}
		if !bytes.Equal(gotS.GetId(), s.GetId()) || gotS.GetRuntimeId() != s.GetRuntimeId() || gotS.GetAlias() != s.GetAlias() || gotS.GetName() != s.GetName() || !gotS.GetListed() || gotS.GetStatus().GetVendorId() != s.GetStatus().GetVendorId() || gotS.GetStatus().GetState() != "stopped" {
			t.Fatal(phase, "session identity or saved conversation changed", gotS)
		}
		gotHistory, err := stack.Session().History(ctx, historyRequest)
		if err != nil || !proto.Equal(gotHistory, history) {
			t.Fatal(phase, "existing history unavailable", gotHistory, err)
		}
	}
	if _, err := stack.Session().Resume(ctx, resource.SessionControl_builder{Ref: historyRequest.GetRef(), RunId: ptr("run"), ClientId: ptr("resume")}.Build()); err != nil {
		t.Fatal("cannot resume the retained session", err)
	}
	if f.resume == nil || f.resume.SessionId != "session" {
		t.Fatal("resume did not reach the original runtime session", f.resume)
	}
}
