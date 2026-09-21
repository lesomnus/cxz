package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
	"github.com/lesomnus/cxz/internal/supervisor"
	"github.com/lesomnus/payday/config"
)

func projectionFixture(t *testing.T) (*Server, core.Session, *journal.Log) {
	t.Helper()
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: ":memory:", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, sql := range []string{"CREATE TABLE sessions(id TEXT PRIMARY KEY, manifest BLOB)", "CREATE TABLE events(session_id TEXT, seq INTEGER, data BLOB, PRIMARY KEY(session_id,seq))"} {
		if _, err = db.Exec(sql); err != nil {
			t.Fatal(err)
		}
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
	l, err := journal.Open(filepath.Join(core.Dir(s.root, m.ID), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return s, m, l
}

func TestProjectionIncrementalRecoveryAndBackground(t *testing.T) {
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
	if _, err := l.AppendBatch(batch); err != nil {
		t.Fatal(err)
	}
	page, err := s.History(ctx, &api.WatchRequest{SessionId: m.ID, AfterSeq: 1875})
	if err != nil || len(page.Events) != 128 || page.Events[0].Seq != 1876 {
		t.Fatal(page, err)
	}
	for _, e := range []core.Event{
		{Kind: "approval_resolved", RequestID: "a", RunID: "run"},
		{Kind: "permission", Text: "ask", RunID: "run"},
		{Kind: "raw", RunID: "run", Raw: []byte(`{"type":"system","subtype":"task_notification","task_id":"bg","status":"completed"}`)},
	} {
		e.SessionID = m.ID
		if _, err = l.Append(e); err != nil {
			t.Fatal(err)
		}
		p, err := s.lockProjection(ctx, m)
		if err != nil {
			t.Fatal(err)
		}
		got := p.snapshot
		p.mu.Unlock()
		if !reflect.DeepEqual(got, supervisor.Replay(l.All())) {
			t.Fatal("incremental replay mismatch", got)
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
	// Restart with an existing DB, then rebuild a lost DB from the same journal.
	for _, rebuild := range []bool{false, true} {
		if rebuild {
			if _, err = s.db.Exec("DELETE FROM events"); err != nil {
				t.Fatal(err)
			}
		}
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
	if err = os.Truncate(filepath.Join(core.Dir(s.root, m.ID), "events.jsonl"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err = s.History(ctx, &api.WatchRequest{SessionId: m.ID}); err == nil || !strings.Contains(err.Error(), "shorter") {
		t.Fatal("lost journal accepted", err)
	}
}

func TestProjectionTransactionFailureRetriesWholeSuffix(t *testing.T) {
	s, m, l := projectionFixture(t)
	ctx := context.Background()
	l.Append(core.Event{SessionID: m.ID, Kind: "state", Text: "idle"})
	if _, err := s.History(ctx, &api.WatchRequest{SessionId: m.ID}); err != nil {
		t.Fatal(err)
	}
	l.AppendBatch([]core.Event{{SessionID: m.ID, Kind: "permission", Text: "ask"}, {SessionID: m.ID, Kind: "assistant", Text: "retry"}})
	if _, err := s.db.Exec("CREATE TRIGGER reject_event BEFORE INSERT ON events WHEN NEW.seq=3 BEGIN SELECT RAISE(ABORT, 'fixture'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.History(ctx, &api.WatchRequest{SessionId: m.ID}); err == nil {
		t.Fatal("transaction failure ignored")
	}
	if s.projections[m.ID].cursor.Seq != 1 || s.projections[m.ID].snapshot.PermissionMode != "full" {
		t.Fatal("uncommitted cursor advanced")
	}
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if count != 1 {
		t.Fatal("partial transaction", count)
	}
	s.db.Exec("DROP TRIGGER reject_event")
	page, err := s.History(ctx, &api.WatchRequest{SessionId: m.ID})
	if err != nil || len(page.Events) != 3 || s.projections[m.ID].snapshot.PermissionMode != "ask" {
		t.Fatal(page, err)
	}
}
