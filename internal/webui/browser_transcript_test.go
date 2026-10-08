package webui

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/eventwire"
	"github.com/lesomnus/cxz/internal/transcripthistory"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
)

func decodeReplay(b []byte) (*api.Event, error) {
	var e api.Event
	err := json.Unmarshal(b, &e)
	return &e, err
}

// Optional private replay exercises the production index and public RPC over HTTP.
func setupTranscriptReplay(t *testing.T, f *browserFixture) {
	t.Helper()
	path := os.Getenv("CXZ_REPLAY_EVENTS")
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var records []core.Event
	if err = json.Unmarshal(data, &records); err != nil {
		t.Fatal(err)
	}
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: ":memory:", MaxOpenConns: 1}).Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`CREATE TABLE events(session_id TEXT,seq INTEGER,data BLOB,PRIMARY KEY(session_id,seq))`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range records {
		native := &api.Event{SessionId: "session", Seq: e.Seq, TimeMs: e.TimeMS, RunId: e.RunID, RequestId: e.RequestID, Kind: e.Kind, Text: e.Text, Payload: e.Payload}
		if e.Response != nil {
			native.Response = &api.ResponseMetadata{Model: e.Response.Model, Effort: e.Response.Effort, Phase: e.Response.Phase, TurnId: e.Response.TurnID, ModelSource: e.Response.ModelSource, EffortSource: e.Response.EffortSource, CompletionJson: []byte(e.Response.CompletionJSON)}
		}
		b, _ := json.Marshal(native)
		if _, err = tx.Exec("INSERT INTO events VALUES(?,?,?)", "session", e.Seq, b); err != nil {
			t.Fatal(err)
		}
		f.events = append(f.events, eventwire.ToResource(native))
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = transcripthistory.Sync(context.Background(), db, "session", decodeReplay); err != nil {
		t.Fatal(err)
	}
	f.transcriptDB = db
	f.resolved = true
}
func (f *browserFixture) Transcript(ctx context.Context, r *resource.SessionTranscriptRequest) (*resource.SessionTranscriptReply, error) {
	if _, err := f.History(ctx, resource.SessionEventsRequest_builder{}.Build()); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	query := &api.TranscriptRequest{SessionId: "session", AfterSeq: r.GetAfterSeq(), BeforeSeq: r.GetBeforeSeq(), Limit: r.GetLimit(), SnapshotSeq: r.GetSnapshotSeq()}
	var page *api.TranscriptReply
	var err error
	if f.transcriptDB != nil {
		page, err = transcripthistory.Page(ctx, f.transcriptDB, query, decodeReplay)
	} else {
		native := make([]*api.Event, 0, len(f.events))
		for _, e := range f.events {
			native = append(native, eventwire.FromResource("session", e))
		}
		page, err = transcripthistory.FromEvents(native, query)
	}
	if err != nil {
		return nil, err
	}
	out := resource.SessionTranscriptReply_builder{SnapshotSeq: &page.SnapshotSeq, HasOlder: &page.HasOlder, HasNewer: &page.HasNewer}.Build()
	for _, e := range page.Events {
		out.SetEvents(append(out.GetEvents(), eventwire.ToResource(e)))
	}
	for _, e := range page.Metadata {
		out.SetMetadata(append(out.GetMetadata(), eventwire.ToResource(e)))
	}
	if page.PrecedingInput != nil {
		out.SetPrecedingInput(eventwire.ToResource(page.PrecedingInput))
	}
	return out, nil
}
func (f *browserFixture) EventDetails(ctx context.Context, r *resource.SessionEventDetailsRequest) (*resource.SessionEventBatch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var page *api.EventBatch
	var err error
	if f.transcriptDB != nil {
		page, err = transcripthistory.Details(ctx, f.transcriptDB, &api.EventDetailsRequest{SessionId: "session", Seq: r.GetSeq()}, decodeReplay)
	} else {
		native := make([]*api.Event, 0, len(f.events))
		for _, e := range f.events {
			native = append(native, eventwire.FromResource("session", e))
		}
		page, err = transcripthistory.DetailsFromEvents(native, r.GetSeq())
	}
	if err != nil {
		return nil, err
	}
	out := resource.SessionEventBatch_builder{}.Build()
	for _, e := range page.Events {
		out.SetEvents(append(out.GetEvents(), eventwire.ToResource(e)))
	}
	return out, nil
}
