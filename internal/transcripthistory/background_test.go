package transcripthistory

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/protobuf/proto"
)

func TestBackgroundTelemetryDoesNotProduceConversationRows(t *testing.T) {
	db, add := fixture(t)
	ctx := context.Background()
	records := []*api.Event{
		event(1, "background", "", `{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[]}`),
		event(2, "input", "", ""),
		event(3, "tool_call", "shell", `{"item":{"type":"commandExecution","command":"sleep 1","status":"inProgress"}}`),
		event(4, "background", "", `{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[{"task_id":"bg","tool_use_id":"shell"}]}`),
		event(5, "tool_result", "shell", `{"item":{"status":"completed","exitCode":0}}`),
		event(6, "assistant", "", ""),
		event(7, "background", "", `{"type":"background","provider":"codex","subtype":"background_status","status":"error"}`),
		event(8, "background", "", `{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[]}`),
	}
	for _, e := range records {
		if e.Kind == "background" {
			e.Text = text(object(e.Payload), "subtype")
		}
	}
	var cache Cache
	// Exercise both the first reduction and subsequent live updates.
	for _, batch := range [][]*api.Event{records[:4], records[4:]} {
		add(batch...)
		if err := SyncWith(ctx, db, "s", decode, &cache); err != nil {
			t.Fatal(err)
		}
	}
	page, err := Page(ctx, db, &api.TranscriptRequest{SessionId: "s"}, decode)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 3 || page.Events[0].Seq != 2 || page.Events[1].Seq != 3 || page.Events[2].Seq != 6 || page.SnapshotSeq != 8 {
		t.Fatal("telemetry changed the visible conversation", page)
	}
	if page.Events[1].GetToolSummary().GetState() != "completed" {
		t.Fatal("the real shell task lost its result", page.Events[1])
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM transcript_rows WHERE kind='background'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("telemetry was materialized as conversation rows", count, err)
	}
	rows, err := Tail(ctx, db, "s", 0, 100, decode)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Event.Kind == "background" {
			t.Fatal("telemetry leaked into the live row stream", row)
		}
	}
	// The WASM sandbox uses the same rules without a database.
	memory, err := FromEvents(records, &api.TranscriptRequest{})
	if err != nil || !proto.Equal(page, memory) {
		t.Fatal("in-memory and persisted conversations disagree", memory, err)
	}
	for _, row := range VersionsFromEvents(records, 0) {
		if row.Event.Kind == "background" {
			t.Fatal("telemetry leaked into the sandbox stream", row)
		}
	}
	details, err := Details(ctx, db, &api.EventDetailsRequest{SessionId: "s", Seq: 4}, decode)
	if err != nil || len(details.Events) != 1 || !proto.Equal(details.Events[0], records[3]) {
		t.Fatal("the original telemetry record was altered", details, err)
	}
}

func TestLegacyBackgroundRowsDoNotConsumePagesOrStreamSlots(t *testing.T) {
	db, add := fixture(t)
	ctx := context.Background()
	records := []*api.Event{
		event(1, "background", "", "{}"),
		event(2, "input", "", ""),
		event(3, "background", "", "{}"),
		event(4, "assistant", "", ""),
		event(5, "background", "", "{}"),
	}
	add(records...)
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}
	// A server opens stores read-only, including ones still written by an old
	// supervisor. Simulate its already materialized rows without rebuilding.
	for _, e := range records {
		if e.Kind != "background" {
			continue
		}
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT OR REPLACE INTO transcript_rows VALUES(?,?,?,?,?)`, "s", e.Seq, e.Seq, e.Kind, b); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE transcript_format SET version=5 WHERE session_id='s'`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name          string
		before, after uint64
		seq           uint64
		older, newer  bool
	}{
		{name: "latest", seq: 4, older: true},
		{name: "older", before: 4, seq: 2, newer: true},
		{name: "newer", after: 2, seq: 4, older: true},
		{name: "only hidden rows remain", after: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := Page(ctx, db, &api.TranscriptRequest{SessionId: "s", BeforeSeq: tc.before, AfterSeq: tc.after, Limit: 1}, decode)
			if err != nil {
				t.Fatal(err)
			}
			if tc.seq == 0 {
				if len(page.Events) != 0 {
					t.Fatal("hidden rows filled the page", page)
				}
			} else if len(page.Events) != 1 || page.Events[0].Seq != tc.seq {
				t.Fatal("hidden rows consumed the page limit", page)
			}
			if page.SnapshotSeq != 5 || page.HasOlder != tc.older || page.HasNewer != tc.newer {
				t.Fatal("hidden rows changed the snapshot or pagination flags", page)
			}
		})
	}
	for _, tc := range []struct{ cursor, seq uint64 }{{0, 2}, {2, 4}, {4, 0}} {
		rows, err := Tail(ctx, db, "s", tc.cursor, 1, decode)
		if err != nil {
			t.Fatal(err)
		}
		if tc.seq == 0 {
			if len(rows) != 0 {
				t.Fatal("hidden rows kept the stream busy", rows)
			}
		} else if len(rows) != 1 || rows[0].Event.Seq != tc.seq || rows[0].Updated != tc.seq {
			t.Fatal("hidden rows consumed the stream limit", rows)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transcript_rows WHERE kind='background'`).Scan(&count); err != nil || count != 3 {
		t.Fatal("reading required a store rewrite", count, err)
	}
	// The writer's normal format upgrade also removes obsolete derived rows.
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM transcript_rows WHERE kind='background'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("the upgraded projection kept legacy telemetry rows", count, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&count); err != nil || count != len(records) {
		t.Fatal("the upgrade removed original records", count, err)
	}
}
