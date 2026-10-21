package workspace

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/historypage"
	"github.com/lesomnus/cxz/internal/sessionstore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type historySource struct {
	api.SessionsClient
	calls, backgrounds int
	through            uint64
	floors             []string
}

func (s *historySource) History(_ context.Context, r *api.WatchRequest, _ ...grpc.CallOption) (*api.EventBatch, error) {
	s.calls++
	b := &api.EventBatch{}
	for seq := r.AfterSeq + 1; seq <= r.AfterSeq+uint64(historypage.PageSize(r.Limit)); seq++ {
		b.Events = append(b.Events, &api.Event{SessionId: r.SessionId, Seq: seq, Text: "event"})
	}
	return b, nil
}
func (s *historySource) GetHistoryFloor(_ context.Context, r *api.SessionRef, _ ...grpc.CallOption) (*api.HistoryFloorReply, error) {
	s.floors = append(s.floors, r.Id)
	return &api.HistoryFloorReply{Through: s.through}, nil
}

func (s *historySource) Background(context.Context, *api.SessionRef, ...grpc.CallOption) (*api.BackgroundReply, error) {
	s.backgrounds++
	return &api.BackgroundReply{LastSeq: 384, Data: []byte(`{}`)}, nil
}

// Raw history is the session's own store, read where its project keeps state
// inside the manager's volume. The manager used to mirror every page into its
// own table, ask the project runtime for anything the mirror could not
// complete, and serve the mirror when the runtime did not answer; none of that
// is left, so this holds with no Docker on PATH and no runtime at all.
// See cxz#268.
func TestHistoryReadsTheSessionsOwnStore(t *testing.T) {
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
	m.StateVolume = "cxz-owner-state"
	p := &Project{ID: strings.Repeat("p", 24), Sessions: []*api.Session{{Id: strings.Repeat("s", 24)}}}
	session := p.Sessions[0].Id
	if err = m.save(ctx, p); err != nil {
		t.Fatal(err)
	}
	// Nothing written yet is an empty page, not an error: the supervisor
	// creates the store when the session starts.
	page, err := m.History(ctx, &api.WatchRequest{SessionId: session})
	if err != nil || len(page.Events) != 0 {
		t.Fatal("a session with no store was not empty", page, err)
	}

	dir, _ := m.projectState(p.ID)
	sessionDir := core.Dir(filepath.Join(dir, "data"), session)
	if err = os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := sessionstore.Open(ctx, sessionstore.Path(sessionDir), session, true)
	if err != nil {
		t.Fatal(err)
	}
	var events []core.Event
	for seq := uint64(1); seq <= 200; seq++ {
		events = append(events, core.Event{SessionID: session, RunID: "run", Seq: seq, Kind: "assistant", Text: "said"})
	}
	if err = store.Append(ctx, events); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	// No Docker, no runtime: any attempt to reach a project container fails.
	t.Setenv("PATH", t.TempDir())
	for _, after := range []uint64{0, 128} {
		page, err := m.History(ctx, &api.WatchRequest{SessionId: session, AfterSeq: after})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Events) == 0 || page.Events[0].Seq != after+1 {
			t.Fatal("the page does not start where it was asked to", len(page.Events), page.Events[0].Seq)
		}
		if page.Events[0].Kind != "assistant" || page.Events[0].Text != "said" {
			t.Fatal("the event is not what the supervisor wrote", page.Events[0])
		}
	}
	// A session the manager does not know is an error rather than silence.
	if _, err = m.History(ctx, &api.WatchRequest{SessionId: strings.Repeat("x", 24)}); err == nil {
		t.Fatal("an unknown session returned a conversation")
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
	b := cachedPage(t, db, "s", 0)
	if len(b) != 7 || b[0].Seq != 15 || b[0].Kind != "history_trimmed" {
		t.Fatal("deleted history resurrected or missing floor", b)
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

// A manager asks the project runtime how far it has trimmed, so that its cache
// stops offering history the journal no longer has. The ask goes over the
// resource API, because that is what the connection to a project container
// speaks: #139 called the call runtime-only and left this caller addressing the
// Docker envelope, which by then no longer carried it, so the read returned an
// error that was swallowed and the floor was never recorded.
func TestTheTrimFloorReachesTheCache(t *testing.T) {
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
	source := &historySource{through: 200}
	m.historyClients = map[string]*historyConnection{p.ID: {
		project: p.ID, container: p.ContainerID, token: p.Token, conn: conn, client: source,
	}}
	t.Setenv("PATH", t.TempDir())

	m.refreshHistoryFloor(ctx, "s")
	if len(source.floors) != 1 || source.floors[0] != "s" {
		t.Fatal("the runtime was not asked", source.floors)
	}
	// The floor is written to the cache as the marker a reader sees in place of
	// what is gone.
	cached := cachedPage(t, db, "s", 199)
	for _, v := range cached {
		if v.Kind == core.HistoryTrimmedKind && core.HistoryFloor(v.Kind, v.Payload) == 200 {
			return
		}
	}
	t.Fatal("the floor was never recorded", cached)
}
