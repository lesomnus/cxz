package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/enginemode"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type engineFixture struct {
	fixture
	saved   []*api.EngineSpec
	started []*api.StartEngineInput
	floors  []string
}

func (f *engineFixture) SaveEngine(_ context.Context, r *api.SaveEngineInput) (*api.EngineReply, error) {
	f.saved = append(f.saved, r.Spec)
	return &api.EngineReply{Status: "saved"}, nil
}

func (f *engineFixture) StartEngine(_ context.Context, r *api.StartEngineInput) (*api.EngineReply, error) {
	f.started = append(f.started, r)
	return &api.EngineReply{Status: "unix:///endpoint"}, nil
}

func (f *engineFixture) GetEngineInfo(context.Context, *api.Empty) (*api.EngineInfo, error) {
	return &api.EngineInfo{Mode: enginemode.Dind, State: "running", Image: "docker:29-dind"}, nil
}

func (f *engineFixture) GetInstallationVersion(context.Context, *api.Empty) (*api.InstallationVersion, error) {
	return &api.InstallationVersion{Version: "v0.1.2", Channel: "stable", Pin: "v0.1.2"}, nil
}

func (f *engineFixture) GetHistoryFloor(_ context.Context, r *api.SessionRef) (*api.HistoryFloorReply, error) {
	f.floors = append(f.floors, r.Id)
	return &api.HistoryFloorReply{Through: 42}, nil
}

func engineStack(t *testing.T) (*engineFixture, resource.Server, func()) {
	t.Helper()
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f := &engineFixture{fixture: fixture{
		p: &api.Project{Id: "project", Workspace: "/work", Name: "work", State: "running"},
		s: &api.Session{
			Id: "session", ProjectId: "project", Workspace: "/work", Title: "a conversation",
			Agent: "claude", Account: "work", AuthBackend: accounts.ProjectLocalOAuth,
			State: "idle", RunId: "run", CreateId: "create",
		},
	}}
	f.s.AuthBinding = accounts.BindingID("project", "work", accounts.ProjectLocalOAuth)
	stack, err := Build(ctx, db, f)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err = stack.Session().List(ctx, &resource.SessionListRequest{}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return f, stack, func() { db.Close() }
}

// A save needs a configuration; a start may apply the one already saved. The
// envelope had one spec field for five operations, so there was nothing to
// check -- an empty payload meant "fall back" for up and "save nothing" for
// save.
func TestSaveNeedsAConfigurationAndStartDoesNot(t *testing.T) {
	ctx := context.Background()
	f, stack, done := engineStack(t)
	defer done()

	if _, err := stack.Project().SaveEngine(ctx, resource.SaveEngineRequest_builder{}.Build()); status.Code(err) != codes.InvalidArgument {
		t.Fatal("saved nothing in particular:", err)
	}
	if len(f.saved) != 0 {
		t.Fatal("it reached the runtime anyway:", f.saved)
	}

	out, err := stack.Project().StartEngine(ctx, resource.StartEngineRequest_builder{}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.started) != 1 || f.started[0].Spec != nil {
		t.Fatal("an absent spec did not stay absent:", f.started)
	}
	if out.GetStatus() != "unix:///endpoint" {
		t.Fatal(out.GetStatus())
	}
}

// The mode is an enum here and a name past this layer, so a value this build
// does not know is refused rather than saved as a mode it is not.
func TestEngineModeIsRefusedAtTheBoundary(t *testing.T) {
	ctx := context.Background()
	f, stack, done := engineStack(t)
	defer done()

	_, err := stack.Project().SaveEngine(ctx, resource.SaveEngineRequest_builder{
		Spec: resource.EngineSpec_builder{Mode: ptr(resource.EngineMode_ENGINE_MODE_UNSPECIFIED)}.Build(),
	}.Build())
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal("an unknown mode was passed on:", err)
	}

	if _, err = stack.Project().SaveEngine(ctx, resource.SaveEngineRequest_builder{
		Spec: resource.EngineSpec_builder{
			Mode: ptr(resource.EngineMode_ENGINE_MODE_DIND), Image: ptr("docker:29-dind"),
			Override: []byte(`{"services":{}}`),
		}.Build(),
	}.Build()); err != nil {
		t.Fatal(err)
	}
	if len(f.saved) != 1 || f.saved[0].Mode != enginemode.Dind || f.saved[0].Image != "docker:29-dind" {
		t.Fatal("the configuration did not arrive as a name:", f.saved)
	}
	// A Compose override is the user's own document and is passed through.
	if string(f.saved[0].Override) != `{"services":{}}` {
		t.Fatal("the override was rewritten:", string(f.saved[0].Override))
	}

	// And it comes back as the enum rather than whatever string the engine holds.
	info, err := stack.Project().GetEngineInfo(ctx, resource.EngineRequest_builder{}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if info.GetMode() != resource.EngineMode_ENGINE_MODE_DIND {
		t.Fatal("the mode did not come back:", info.GetMode())
	}
}

// What cxz is was answered inside the engine's info, which is how a question
// about Docker came to answer a question about cxz. It is its own call, and it
// does not report an engine.
func TestTheBuildIsNotTheEnginesBusiness(t *testing.T) {
	ctx := context.Background()
	_, stack, done := engineStack(t)
	defer done()
	out, err := stack.Project().GetInstallationVersion(ctx, resource.EngineRequest_builder{}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if out.GetVersion() != "v0.1.2" || out.GetChannel() != "stable" || out.GetPin() != "v0.1.2" {
		t.Fatal(out)
	}
}

// The floor is read over this API, because this is what the connection between
// a manager and a project container speaks. It was declared runtime-only in
// #139 and its one caller was left on the Docker envelope, which by then no
// longer carried it -- so the read silently did nothing.
func TestHistoryFloorIsReadableThroughTheResourceAPI(t *testing.T) {
	ctx := context.Background()
	f, stack, done := engineStack(t)
	defer done()
	out, err := stack.Session().GetHistoryFloor(ctx, resource.SessionHistoryFloorRequest_builder{
		Ref: sessionRef("session"),
	}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if out.GetThrough() != 42 {
		t.Fatal("the floor did not come back:", out.GetThrough())
	}
	if len(f.floors) != 1 || f.floors[0] != "session" {
		t.Fatal("the ref was not resolved:", f.floors)
	}
}
