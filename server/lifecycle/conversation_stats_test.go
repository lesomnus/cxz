package lifecycle

import (
	"context"
	"encoding/hex"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"net"
	"path/filepath"
	"testing"
	"time"
)

type statsFixture struct {
	*fixture
	analytics      bool
	analyticsError error
	scope, id      string
	historyCalls   int
}

func (f *statsFixture) TelemetryStats(_ context.Context, scope, id string, from, to int64) (*resource.ConversationStatsReply, bool, error) {
	if !f.analytics {
		return nil, false, nil
	}
	f.scope, f.id = scope, id
	if f.analyticsError != nil {
		return nil, true, f.analyticsError
	}
	return resource.ConversationStatsReply_builder{FromMs: &from, ToMs: &to, Total: resource.ConversationStatsValues_builder{InputTokens: ptr(uint64(321))}.Build(), Complete: ptr(false)}.Build(), true, nil
}

func (f *statsFixture) ResourceSnapshot(context.Context) (*api.ProjectList, *api.SessionList, error) {
	p := &api.Project{Id: "other-project", Workspace: "/other", State: "running"}
	other := &api.Session{Id: "abcdef0123456789abcdef01", ProjectId: p.Id, Agent: "codex", Account: "personal", AuthBackend: accounts.ProjectLocalOAuth, AuthBinding: accounts.BindingID(p.Id, "personal", accounts.ProjectLocalOAuth), CreateId: "other-create", State: "idle"}
	return &api.ProjectList{Projects: []*api.Project{f.p, p}}, &api.SessionList{Sessions: []*api.Session{f.s, other}}, nil
}

// Statistics never read conversation history; counting the calls proves it.
func (f *statsFixture) History(context.Context, *api.WatchRequest) (*api.EventBatch, error) {
	f.historyCalls++
	return &api.EventBatch{}, nil
}

func TestConversationStatsRPC(t *testing.T) {
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	f := &statsFixture{fixture: &fixture{p: &api.Project{Id: "project", Workspace: "/workspace", Name: "project", State: "running"}, s: &api.Session{Id: "0123456789abcdef01234567", ProjectId: "project", Agent: "codex", CreateId: "create", RunId: "run", State: "idle", LastSeq: 4}}}
	f.s.Account = "work"
	f.s.AuthBackend = accounts.ProjectLocalOAuth
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	ln := bufconn.Listen(1 << 20)
	g := grpc.NewServer()
	resource.RegisterServer(g, stack)
	go g.Serve(ln)
	defer g.Stop()
	conn, err := grpc.NewClient("passthrough:///test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := resource.NewSessionServiceClient(conn)
	request := func(scope string) *resource.ConversationStatsRequest {
		r := resource.ConversationStatsRequest_builder{FromMs: ptr(day), ToMs: ptr(day + 86400000)}.Build()
		switch scope {
		case "project":
			r.SetProject(projectRef("project"))
		case "session":
			r.SetSession(sessionRef(f.s.Id))
		case "account":
			r.SetAccount(accountRef("work"))
		}
		return r
	}
	// Without the telemetry store there is no usage, rather than a second answer.
	for _, scope := range []string{"all", "project", "session", "account"} {
		if _, err := c.ConversationStats(ctx, request(scope)); status.Code(err) != codes.Unavailable {
			t.Fatal(scope, "answered without telemetry", err)
		}
	}
	f.analytics = true
	ids := map[string]string{"all": "", "project": hex.EncodeToString(projectid.Bytes("project")), "session": hex.EncodeToString(resourceID(8, f.s.Id)), "account": hex.EncodeToString(resourceID(9, "work"))}
	for _, scope := range []string{"all", "project", "session", "account"} {
		got, err := c.ConversationStats(ctx, request(scope))
		want := scope
		if scope == "all" {
			want = ""
		}
		if err != nil || got.GetTotal().GetInputTokens() != 321 || got.GetComplete() || f.scope != want || f.id != ids[scope] {
			t.Fatal(scope, got, err, f.scope, f.id)
		}
	}
	f.analyticsError = status.Error(codes.Unavailable, "telemetry database unavailable")
	if _, err = c.ConversationStats(ctx, request("account")); status.Code(err) != codes.Unavailable {
		t.Fatal("telemetry failure was not reported", err)
	}
	f.analyticsError = nil
	if f.historyCalls != 0 {
		t.Fatal("statistics read conversation history", f.historyCalls)
	}
	_, err = c.ConversationStats(ctx, resource.ConversationStatsRequest_builder{Account: accountRef("missing")}.Build())
	if status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
	_, err = c.ConversationStats(ctx, resource.ConversationStatsRequest_builder{Account: accountRef("work"), Project: projectRef("project")}.Build())
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	_, err = c.ConversationStats(ctx, resource.ConversationStatsRequest_builder{Project: projectRef("project"), Session: sessionRef(f.s.Id)}.Build())
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	_, err = c.ConversationStats(ctx, resource.ConversationStatsRequest_builder{Project: projectRef("missing")}.Build())
	if status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
	_, err = c.ConversationStats(ctx, resource.ConversationStatsRequest_builder{FromMs: ptr(day), ToMs: ptr(day)}.Build())
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}
