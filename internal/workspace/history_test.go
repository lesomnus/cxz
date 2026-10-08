package workspace

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/historypage"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

type historySource struct {
	api.SessionsClient
	calls, backgrounds int
}

func (s *historySource) History(_ context.Context, r *api.WatchRequest, _ ...grpc.CallOption) (*api.EventBatch, error) {
	s.calls++
	b := &api.EventBatch{}
	for seq := r.AfterSeq + 1; seq <= r.AfterSeq+uint64(historypage.PageSize(r.Limit)); seq++ {
		b.Events = append(b.Events, &api.Event{SessionId: r.SessionId, Seq: seq, Text: "event"})
	}
	return b, nil
}
func (s *historySource) Docker(context.Context, *api.DockerInput, ...grpc.CallOption) (*api.Receipt, error) {
	return &api.Receipt{Status: `{"through":0}`}, nil
}

func (s *historySource) Background(context.Context, *api.SessionRef, ...grpc.CallOption) (*api.BackgroundReply, error) {
	s.backgrounds++
	return &api.BackgroundReply{LastSeq: 384, Data: []byte(`{}`)}, nil
}

func TestHistoryCacheCompletenessAndTransportReuse(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE events(session_id TEXT, seq INTEGER, data BLOB, PRIMARY KEY(session_id,seq))"); err != nil {
		t.Fatal(err)
	}
	m, err := New(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	p := &Project{ID: "p", ContainerID: "container", Token: "token", Sessions: []*api.Session{{Id: "s"}}}
	if err = m.save(ctx, p); err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient("passthrough:///unused", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	source := &historySource{}
	c := &historyConnection{project: p.ID, container: p.ContainerID, token: p.Token, conn: conn, client: source}
	m.historyClients = map[string]*historyConnection{p.ID: c}
	// Any unexpected Docker invocation fails. Already connected readers and
	// complete cached ranges must remain usable without spawning a subprocess.
	t.Setenv("PATH", t.TempDir())
	for _, after := range []uint64{0, 128, 0, 128} {
		page, err := m.History(ctx, &api.WatchRequest{SessionId: "s", AfterSeq: after})
		if err != nil || len(page.Events) != 128 || page.Events[0].Seq != after+1 {
			t.Fatal(page, err)
		}
	}
	if source.calls != 2 || m.historyClients[p.ID] != c {
		t.Fatal("range re-fetched or connection replaced", source.calls)
	}
	// 128 cached rows with a missing sequence are not a complete cached page.
	gap := &api.EventBatch{}
	for seq := uint64(257); seq <= 385; seq++ {
		if seq != 260 {
			gap.Events = append(gap.Events, &api.Event{SessionId: "s", Seq: seq})
		}
	}
	if err = m.cache(ctx, gap); err != nil {
		t.Fatal(err)
	}
	page, err := m.History(ctx, &api.WatchRequest{SessionId: "s", AfterSeq: 256})
	if err != nil || source.calls != 3 || len(page.Events) != 128 || page.Events[3].Seq != 260 {
		t.Fatal("gap skipped", page, err)
	}
	for range 2 {
		if _, err = m.Background(ctx, &api.SessionRef{Id: "s"}); err != nil {
			t.Fatal(err)
		}
	}
	if source.backgrounds != 2 || m.historyClients[p.ID] != c {
		t.Fatal("snapshot did not reuse transport")
	}
	for _, limit := range []uint32{1024, 1024, ^uint32(0), 0} {
		page, err := m.History(ctx, &api.WatchRequest{SessionId: "s", Limit: limit})
		if err != nil || len(page.Events) != historypage.PageSize(limit) || source.calls != 4 {
			t.Fatal("larger page was treated as complete too early or re-fetched", len(page.Events), source.calls, err)
		}
	}
	p.ContainerID = "replacement"
	if err = m.save(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err = m.historyClient(ctx, "s"); err == nil || conn.GetState() != connectivity.Shutdown || m.historyClients[p.ID] != nil {
		t.Fatal("replacement reused stale capability/endpoint", err)
	}
	// The immutable range remains readable even with the runtime unavailable.
	if page, err = m.History(ctx, &api.WatchRequest{SessionId: "s"}); err != nil || len(page.Events) != 128 {
		t.Fatal(page, err)
	}
}

func TestHistoryCacheTrimCannotResurrectOldRows(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("CREATE TABLE events(session_id TEXT,seq INTEGER,data BLOB,PRIMARY KEY(session_id,seq))")
	m := &Manager{DB: db}
	initial := &api.EventBatch{}
	for i := uint64(1); i <= 20; i++ {
		initial.Events = append(initial.Events, &api.Event{SessionId: "s", Seq: i, Kind: "assistant", Text: "old"})
	}
	if err = m.cache(t.Context(), initial); err != nil {
		t.Fatal(err)
	}
	notice := &api.Event{SessionId: "s", Seq: 21, Kind: "history_trimmed", Payload: []byte(`{"through":15}`)}
	if err = m.cache(t.Context(), &api.EventBatch{Events: []*api.Event{notice}}); err != nil {
		t.Fatal(err)
	}
	if err = m.cache(t.Context(), initial); err != nil {
		t.Fatal(err)
	} // in-flight old response
	b, err := m.cachedHistory(t.Context(), &api.WatchRequest{SessionId: "s"})
	if err != nil || len(b.Events) != 7 || b.Events[0].Seq != 15 || b.Events[0].Kind != "history_trimmed" {
		t.Fatal("deleted history resurrected or missing floor", b, err)
	}
	// A stale checkpoint is also forbidden from restoring a lower floor.
	old := &api.Event{SessionId: "s", Seq: 5, Kind: "history_trimmed", Payload: []byte(`{"through":5}`)}
	if err = m.cache(t.Context(), &api.EventBatch{Events: []*api.Event{old}}); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow("SELECT COUNT(*) FROM events WHERE seq<15").Scan(&n)
	if n != 0 {
		t.Fatal("stale boundary inserted")
	}
}
