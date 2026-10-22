package server

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionstore"
	"github.com/lesomnus/cxz/internal/supervisor"
	"github.com/lesomnus/payday/config"
)

// recordingLog is a session as the supervisor runs one: every append is
// followed by the state those records reduce to, which is what a reader takes
// instead of replaying anything. The tests append; this keeps the pair
// together, exactly as `Supervisor.event` does. See cxz#278.
type recordingLog struct {
	*sessionstore.Log
	t *testing.T
}

func (l *recordingLog) Append(e core.Event) (core.Event, error) {
	out, err := l.Log.Append(e)
	if err == nil {
		l.recordState()
	}
	return out, err
}

func (l *recordingLog) AppendBatch(batch []core.Event) ([]core.Event, error) {
	out, err := l.Log.AppendBatch(batch)
	if err == nil {
		l.recordState()
	}
	return out, err
}

func (l *recordingLog) recordState() {
	l.t.Helper()
	if err := l.Log.Store().SetState(context.Background(), supervisor.Replay(l.Log.All())); err != nil {
		l.t.Fatal(err)
	}
}

// The fixture is a session that records the way a session does: into its own
// store, with the reduced state written beside the records. The server reads
// it and never writes it. See cxz#278.
func projectionFixture(t *testing.T) (*Server, core.Session, *recordingLog) {
	t.Helper()
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: ":memory:", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec("CREATE TABLE sessions(id TEXT PRIMARY KEY, manifest BLOB)"); err != nil {
		t.Fatal(err)
	}
	s := &Server{root: t.TempDir(), db: db}
	m := core.Session{ID: strings.Repeat("a", 24), Kind: "claude"}
	b, _ := json.Marshal(m)
	if _, err = db.Exec("INSERT INTO sessions VALUES(?,?)", m.ID, b); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(core.Dir(s.root, m.ID), 0700); err != nil {
		t.Fatal(err)
	}
	l, err := sessionstore.OpenLog(ctx, core.Dir(s.root, m.ID), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return s, m, &recordingLog{Log: l, t: t}
}

func record(t *testing.T, l *recordingLog, events ...core.Event) {
	t.Helper()
	if _, err := l.AppendBatch(events); err != nil {
		t.Fatal(err)
	}
}

func TestProjectionFollowsTheRecordAndItsBackground(t *testing.T) {
	s, m, l := projectionFixture(t)
	ctx := context.Background()
	batch := []core.Event{
		{Kind: "state", Text: "working", RunID: "run"},
		{Kind: "approval", RequestID: "a", RunID: "run"},
		{Kind: "raw", RunID: "run", Raw: []byte(`{"type":"system","subtype":"task_started","task_id":"bg","is_backgrounded":true}`)},
	}
	for i := 0; i < 2000; i++ {
		batch = append(batch, core.Event{Kind: "assistant", RunID: "run", Text: "history"})
	}
	for i := range batch {
		batch[i].SessionID = m.ID
	}
	record(t, l, batch...)

	page, err := s.History(ctx, &api.WatchRequest{SessionId: m.ID, AfterSeq: 1875})
	if err != nil || len(page.Events) != 128 || page.Events[0].Seq != 1876 {
		t.Fatal(page, err)
	}
	// The projection follows every further record, because a session keeps
	// recording and a projection taken once would answer with the conversation
	// as it was.
	for _, e := range []core.Event{
		{Kind: "approval_resolved", RequestID: "a", RunID: "run"},
		{Kind: "permission", Text: "ask", RunID: "run"},
		{Kind: "raw", RunID: "run", Raw: []byte(`{"type":"system","subtype":"task_notification","task_id":"bg","status":"completed"}`)},
	} {
		e.SessionID = m.ID
		record(t, l, e)
		p, err := s.lockProjection(ctx, m)
		if err != nil {
			t.Fatal(err)
		}
		got := p.snapshot
		p.mu.Unlock()
		if !reflect.DeepEqual(got, supervisor.Replay(l.All())) {
			t.Fatal("the projection is not what the record reduces to", got)
		}
	}
	state, err := s.Background(ctx, &api.SessionRef{Id: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	var background map[string]*agentview.BackgroundState
	if err = json.Unmarshal(state.Data, &background); err != nil {
		t.Fatal(err)
	}
	if state.LastSeq != 2006 || background["run"].Tasks["bg"].Active || background["run"].Tasks["bg"].Status != "completed" {
		t.Fatal(string(state.Data))
	}

	// A server that has never seen this session answers the same, with no
	// replay and nothing of its own to rebuild: the record is the store.
	restarted := &Server{root: s.root, db: s.db}
	recovered, err := restarted.Background(ctx, &api.SessionRef{Id: m.ID})
	if err != nil || recovered.LastSeq != state.LastSeq || string(recovered.Data) != string(state.Data) {
		t.Fatal(recovered, err)
	}
	page, err = restarted.History(ctx, &api.WatchRequest{SessionId: m.ID, AfterSeq: 2000})
	if err != nil || len(page.Events) != 6 {
		t.Fatal(page, err)
	}
}

// A session with no store at all is a session that has recorded nothing, which
// is what a session looks like for the moment between being created and
// starting. Not an error, and not a conversation either.
func TestASessionThatHasRecordedNothing(t *testing.T) {
	s, m, l := projectionFixture(t)
	ctx := context.Background()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sessionstore.Path(core.Dir(s.root, m.ID))); err != nil {
		t.Fatal(err)
	}
	page, err := s.History(ctx, &api.WatchRequest{SessionId: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 0 {
		t.Fatal("a session that recorded nothing had a conversation", page.Events)
	}
}
