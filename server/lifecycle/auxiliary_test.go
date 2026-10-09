package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"path/filepath"
	"testing"
)

type auxiliaryFixture struct {
	fixture
	models *api.AuxModelsInput
	calls  int
	title  string
}

func (f *auxiliaryFixture) AuxModels(_ context.Context, r *api.AuxModelsInput) (*api.AuxModelsReply, error) {
	f.calls++
	f.models = r
	return &api.AuxModelsReply{}, nil
}
func (f *auxiliaryFixture) AuxStatus(_ context.Context, r *api.AuxStatusInput) (*api.AuxState, error) {
	f.calls++
	return &api.AuxState{Title: f.title}, nil
}
func (f *auxiliaryFixture) AuxEvents(r *api.AuxStatusInput, stream api.Sessions_AuxEventsServer) error {
	f.calls++
	return stream.Send(&api.AuxState{Title: f.title})
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
	// The request names an account and nothing else about authentication; what
	// it authenticates as is read off the registered account.
	_, e = stack.Project().AuxModels(ctx, resource.AuxModelsRequest_builder{Account: ptr("work")}.Build())
	if e != nil {
		t.Fatal(e)
	}
	if f.models.Agent != "codex" || f.models.Backend != accounts.BrokeredAccessToken {
		t.Fatal("wrong authentication for the registered account", f.models)
	}
	_, e = stack.Project().AuxModels(ctx, resource.AuxModelsRequest_builder{Account: ptr("missing")}.Build())
	if e == nil || f.calls != 1 {
		t.Fatal("missing account reached runner")
	}
	_, e = stack.Project().AuxModels(ctx, resource.AuxModelsRequest_builder{}.Build())
	if e == nil || f.calls != 1 {
		t.Fatal("nameless account reached runner")
	}
}

// The pushed answer is the same answer, including recording a generated title
// on the session it names -- so a client that is subscribed does not have to
// ask once more for the name to be right.
func TestAuxEventsRelayRecordsTheTitle(t *testing.T) {
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
	if _, err = stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef("session"), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build()); err != nil {
		t.Fatal(err)
	}
	f.title = "Pushed title"
	sink := &auxRelaySink{ctx: ctx}
	if err = stack.Session().AuxEvents(resource.AuxStatusRequest_builder{Ref: sessionRef("session")}.Build(), sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.sent) != 1 || sink.sent[0].GetTitle() != "Pushed title" {
		t.Fatal("the state was not relayed", sink.sent)
	}
	v, err := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef("session"), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if v.GetName() != "Pushed title" {
		t.Fatal("a pushed title was not recorded", v.GetName())
	}
}

type auxRelaySink struct {
	grpc.ServerStreamingServer[resource.AuxState]
	ctx  context.Context
	sent []*resource.AuxState
}

func (s *auxRelaySink) Context() context.Context { return s.ctx }
func (s *auxRelaySink) Send(v *resource.AuxState) error {
	s.sent = append(s.sent, v)
	return nil
}

// A kind this build does not know is refused at the edge, before it can reach
// the controller as something it would have to interpret.
func TestAuxRefusesAnUnknownKindAtTheEdge(t *testing.T) {
	ctx := t.Context()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &auxiliaryFixture{fixture: fixture{p: &api.Project{Id: "project", Workspace: "/work", Name: "test"}, s: &api.Session{Id: "session", ProjectId: "project", Agent: "codex", LastSeq: 1, Account: "work", AuthBackend: accounts.ProjectLocalOAuth}}}
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	_, err = stack.Session().AuxRun(ctx, resource.AuxRunRequest_builder{
		Ref: sessionRef("session"), Kinds: []resource.AuxKind{resource.AuxKind(99)},
	}.Build())
	if status.Code(err) != codes.InvalidArgument || f.calls != 0 {
		t.Fatal("an unknown kind reached the runner", err, f.calls)
	}
	_, err = stack.Session().AuxPrefer(ctx, resource.AuxPreferRequest_builder{
		Ref:         sessionRef("session"),
		Preferences: []*resource.AuxPreference{resource.AuxPreference_builder{Kind: ptr(resource.AuxKind_AUX_KIND_UNSPECIFIED)}.Build()},
	}.Build())
	if status.Code(err) != codes.InvalidArgument || f.calls != 0 {
		t.Fatal("an unset kind reached the runner", err, f.calls)
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
	// A generated title is a field of the answer, so recording it is not a
	// guess about what an opaque reply contained.
	f.title = "New title"
	if _, err := stack.Session().AuxStatus(ctx, resource.AuxStatusRequest_builder{Ref: sessionRef("session")}.Build()); err != nil {
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
