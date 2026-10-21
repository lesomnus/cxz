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

