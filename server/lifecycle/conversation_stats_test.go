package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
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
	events []*api.Event
}

func (f *statsFixture) ResourceSnapshot(context.Context) (*api.ProjectList, *api.SessionList, error) {
	p := &api.Project{Id: "other-project", Workspace: "/other", State: "running"}
	other := &api.Session{Id: "abcdef0123456789abcdef01", ProjectId: p.Id, Agent: "codex", Account: "work", AuthBackend: accounts.ProjectLocalOAuth, AuthBinding: accounts.BindingID(p.Id, "work", accounts.ProjectLocalOAuth), CreateId: "other-create", State: "idle"}
	return &api.ProjectList{Projects: []*api.Project{f.p, p}}, &api.SessionList{Sessions: []*api.Session{f.s, other}}, nil
}

func (f *statsFixture) History(_ context.Context, r *api.WatchRequest) (*api.EventBatch, error) {
	b := &api.EventBatch{}
	for _, e := range f.events {
		if e.Seq > r.AfterSeq {
			b.Events = append(b.Events, e)
			if len(b.Events) == 2 {
				break
			}
		}
	}
	return b, nil
}

func TestConversationStatsRPC(t *testing.T) {
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	f := &statsFixture{fixture: &fixture{p: &api.Project{Id: "project", Workspace: "/workspace", Name: "project", State: "running"}, s: &api.Session{Id: "0123456789abcdef01234567", ProjectId: "project", Agent: "codex", CreateId: "create", RunId: "run", State: "idle", LastSeq: 4}}, events: []*api.Event{
		{Seq: 1, Kind: "turn_end", RunId: "run", TimeMs: day - 1, Payload: []byte(`{"total_cost_usd":2}`)},
		{Seq: 2, Kind: "input", RunId: "run", TimeMs: day, Text: "한글"},
		{Seq: 3, Kind: "usage", RunId: "run", TimeMs: day, Text: "thread/tokenUsage/updated", Payload: []byte(`{"tokenUsage":{"last":{"inputTokens":100,"cachedInputTokens":40,"outputTokens":10}}}`)},
		{Seq: 4, Kind: "turn_end", RunId: "run", TimeMs: day, Payload: []byte(`{"total_cost_usd":3}`)},
	}}
	f.s.Account = "work"
	f.s.AuthBackend = accounts.ProjectLocalOAuth
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f)
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
	for _, scope := range []string{"all", "project", "session"} {
		r := resource.ConversationStatsRequest_builder{FromMs: ptr(day), ToMs: ptr(day + 86400000)}.Build()
		if scope == "project" {
			r.SetProject(projectRef("project"))
		}
		if scope == "session" {
			r.SetSession(sessionRef(f.s.Id))
		}
		got, err := c.ConversationStats(ctx, r)
		if err != nil {
			t.Fatal(scope, err)
		}
		wantSessions := 1
		if scope == "all" {
			wantSessions = 2
		}
		v := got.GetTotal()
		if !got.GetComplete() || len(got.GetSessions()) != wantSessions || len(got.GetDays()) != 1 || v.GetEvents() != 3 || v.GetConversationBytes() != 6 || v.GetInputTokens() != 60 || v.GetCacheReadTokens() != 40 || v.GetOutputTokens() != 10 || v.GetReportedCostUsd() != 1 || v.GetTokenReportedTurns() != 1 {
			t.Fatalf("%s: %v", scope, got)
		}
	}

	_, err = c.ConversationStats(ctx, resource.ConversationStatsRequest_builder{Project: projectRef("project"), Session: sessionRef(f.s.Id)}.Build())
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	_, err = c.ConversationStats(ctx, resource.ConversationStatsRequest_builder{Project: projectRef("missing")}.Build())
	if status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
	// A pruned/offline cache must never be presented as full lifetime coverage.
	f.events = f.events[2:]
	got, err := c.ConversationStats(ctx, resource.ConversationStatsRequest_builder{FromMs: ptr(day), ToMs: ptr(day + 86400000)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if got.GetComplete() || got.GetTotal().GetCostReportedTurns() != 0 {
		t.Fatal(got)
	}
	_, err = c.ConversationStats(ctx, resource.ConversationStatsRequest_builder{FromMs: ptr(day), ToMs: ptr(day)}.Build())
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}

func TestStatsUsageMissingAndRunIsolation(t *testing.T) {
	s := statsUsage{cost: map[string]float64{}, known: map[string]bool{}}
	s.event(&api.Event{Kind: "usage", RunId: "old", Text: "thread/tokenUsage/updated", Payload: []byte(`{"tokenUsage":{"last":{"inputTokens":500}}}`)})
	got := s.event(&api.Event{Kind: "turn_end", RunId: "new"})
	if got.GetTokenReportedTurns() != 0 || got.GetCostReportedTurns() != 0 {
		t.Fatal(got)
	}
	got = s.event(&api.Event{Kind: "turn_end", RunId: "new", Payload: []byte(`{"usage":{"input_tokens":20,"output_tokens":3,"cache_read_input_tokens":50,"cache_creation_input_tokens":10},"costUSD":0}`)})
	if got.GetInputTokens() != 20 || got.GetCacheReadTokens() != 50 || got.GetCostReportedTurns() != 1 || got.GetReportedCostUsd() != 0 {
		t.Fatal(got)
	}
}
