package transcripthistory

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/payday/config"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
	"google.golang.org/protobuf/proto"
)

func fixture(t *testing.T) (*sql.DB, func(...*api.Event)) {
	t.Helper()
	db, err := func() (*sql.DB, error) {
		db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: ":memory:", MaxOpenConns: 1}).Open(context.Background())
		return db, err
	}()
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`CREATE TABLE events(session_id TEXT,seq INTEGER,data BLOB,PRIMARY KEY(session_id,seq))`); err != nil {
		t.Fatal(err)
	}
	return db, func(events ...*api.Event) {
		for _, e := range events {
			e.SessionId = "s"
			b, _ := json.Marshal(e)
			if _, err := db.Exec("INSERT OR REPLACE INTO events VALUES(?,?,?)", "s", e.Seq, b); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func decode(b []byte) (*api.Event, error) {
	var e api.Event
	err := json.Unmarshal(b, &e)
	return &e, err
}
func event(seq uint64, kind, request string, payload string) *api.Event {
	return &api.Event{Seq: seq, Kind: kind, RunId: "run", RequestId: request, Payload: []byte(payload), TimeMs: int64(seq * 1000)}
}
func syncPage(t *testing.T, db *sql.DB, r *api.TranscriptRequest) *api.TranscriptReply {
	t.Helper()
	ctx := context.Background()
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}
	r.SessionId = "s"
	p, err := Page(ctx, db, r, decode)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestSnapshotPagesAndLazyTaskDetails(t *testing.T) {
	db, add := fixture(t)
	records := []*api.Event{event(1, "input", "", ""), event(2, "tool_call", "task", `{"item":{"type":"commandExecution","command":"/bin/zsh -lc 'git status'","status":"inProgress"}}`), event(3, "approval", "approve", `{"params":{"itemId":"task"}}`), event(4, "approval_resolved", "approve", ""), event(5, "tool_output", "task", `{"content":"full output"}`), event(7, "assistant", "", "")}
	records[3].Text = "allowed"
	records[4].Text = "output"
	add(records...)
	for i := uint64(8); i < 1000; i++ {
		add(event(i, "raw", "", "{}"))
	}
	p := syncPage(t, db, &api.TranscriptRequest{Limit: 2})
	if p.SnapshotSeq != 999 || len(p.Events) != 2 || p.Events[0].Seq != 2 || p.Events[0].ToolSummary.Command != "git status" || p.Events[0].ToolSummary.State != "working" || len(p.Events[0].Payload) != 0 || !p.HasOlder || p.HasNewer || p.PrecedingInput.Seq != 1 {
		t.Fatal(p)
	}
	result := event(1000, "tool_result", "task", `{"item":{"exitCode":0,"aggregatedOutput":"full result"}}`)
	add(result)
	old := syncPage(t, db, &api.TranscriptRequest{SnapshotSeq: 999})
	current := syncPage(t, db, &api.TranscriptRequest{})
	if old.Events[1].ToolSummary.State != "working" || current.Events[1].ToolSummary.State != "completed" || current.Events[1].Seq != 2 {
		t.Fatal(old, current)
	}
	older := syncPage(t, db, &api.TranscriptRequest{BeforeSeq: 2, SnapshotSeq: 999, Limit: 2})
	if len(older.Events) != 1 || older.Events[0].Seq != 1 || older.HasOlder || !older.HasNewer {
		t.Fatal(older)
	}
	// Native details include the original command, output, and approval lifecycle;
	// reused IDs from another run must never be joined.
	other := event(1001, "tool_result", "task", "{}")
	other.RunId = "other"
	add(other)
	syncPage(t, db, &api.TranscriptRequest{})
	details, err := Details(context.Background(), db, &api.EventDetailsRequest{SessionId: "s", Seq: 2}, decode)
	if err != nil || len(details.Events) != 5 || details.Events[0].Seq != 2 || details.Events[4].Seq != 1000 || len(details.Events[0].Payload) == 0 {
		t.Fatal(details, err)
	}
	memory, err := FromEvents(append(records, result, other), &api.TranscriptRequest{SnapshotSeq: 999, Limit: 2})
	if err != nil || !proto.Equal(old.Events[1], memory.Events[0]) {
		t.Fatal(memory, err)
	}
}
func TestBackfillRetentionAndCancellation(t *testing.T) {
	db, add := fixture(t)
	add(event(10, "input", "", ""), event(30, "assistant", "", ""))
	if p := syncPage(t, db, &api.TranscriptRequest{}); len(p.Events) != 2 {
		t.Fatal(p)
	}
	add(event(20, "assistant", "", "")) // fills a hole without changing the high-water mark
	if p := syncPage(t, db, &api.TranscriptRequest{}); len(p.Events) != 3 || p.Events[1].Seq != 20 {
		t.Fatal(p)
	}
	db.Exec("DELETE FROM events WHERE seq<30")
	if p := syncPage(t, db, &api.TranscriptRequest{}); len(p.Events) != 1 || p.HasOlder || p.PrecedingInput != nil {
		t.Fatal(p)
	}
	add(event(40, "assistant", "", ""))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Sync(ctx, db, "s", decode); err == nil {
		t.Fatal("canceled index must not commit")
	}
	if p := syncPage(t, db, &api.TranscriptRequest{}); len(p.Events) != 2 {
		t.Fatal(p)
	}
}
func TestCompletionAndMetadata(t *testing.T) {
	db, add := fixture(t)
	input := event(1, "input", "", "")
	assistant := event(2, "assistant", "", `{"item":{"phase":"final_answer"}}`)
	assistant.Response = &api.ResponseMetadata{Model: "model", Effort: "high", Phase: "final_answer"}
	end := event(3, "turn_end", "", `{"duration_ms":1200,"usage":{"input_tokens":100,"cache_read_input_tokens":20},"total_cost_usd":0.01}`)
	end.Text = "completed"
	add(input, assistant, end, event(4, "models", "", "{}"), event(5, "usage", "", "{}"))
	p := syncPage(t, db, &api.TranscriptRequest{})
	if len(p.Events) != 2 || len(p.Metadata) != 2 || p.Events[1].Response.Model != "model" {
		t.Fatal(p)
	}
	c := object(p.Events[1].Response.CompletionJson)
	if c["duration_source"] != "provider" || c["duration_ms"] != float64(1200) || child(c, "metrics")["cache_read_tokens"] != float64(20) {
		t.Fatal(c)
	}
	// Incremental restore must not attach an empty turn to a previously completed response.
	next := event(6, "turn_end", "", "")
	next.Text = "completed"
	add(next)
	if p = syncPage(t, db, &api.TranscriptRequest{}); len(p.Events) != 3 || p.Events[2].Kind != "turn_end" {
		t.Fatal(p)
	}
	commentary := event(7, "assistant", "", `{"item":{"phase":"commentary"}}`)
	add(commentary)
	next = event(8, "turn_end", "", "")
	next.Text = "completed"
	add(next)
	if p = syncPage(t, db, &api.TranscriptRequest{}); len(p.Events) != 5 || p.Events[4].Kind != "turn_end" {
		t.Fatal(p)
	}
}
