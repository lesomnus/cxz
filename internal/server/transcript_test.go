package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

func TestTranscriptUsesJournalResultsWithoutChangingHistory(t *testing.T) {
	s, m, l := projectionFixture(t)
	ctx := context.Background()
	records := []core.Event{{Kind: "input", Text: "run command", RunID: "run"}, {Kind: "tool_call", RequestID: "task", RunID: "run", Payload: json.RawMessage(`{"item":{"type":"commandExecution","command":"git status","status":"inProgress"}}`)}}
	for i := 0; i < 2000; i++ {
		records = append(records, core.Event{Kind: "raw", RunID: "run", Raw: []byte("provider stream")})
	}
	records = append(records, core.Event{Kind: "tool_result", RequestID: "task", RunID: "run", Text: "full result", Payload: json.RawMessage(`{"item":{"status":"completed","exitCode":0}}`)}, core.Event{Kind: "assistant", RunID: "run", Text: "done"})
	for i := range records {
		records[i].SessionID = m.ID
	}
	if _, err := l.AppendBatch(records); err != nil {
		t.Fatal(err)
	}
	page, err := s.Transcript(ctx, &api.TranscriptRequest{SessionId: m.ID, Limit: 256})
	if err != nil || len(page.Events) != 3 || page.SnapshotSeq != 2004 || page.Events[1].Seq != 2 || page.Events[1].ToolSummary.State != "completed" {
		t.Fatal(page, err)
	}
	raw, err := s.History(ctx, &api.WatchRequest{SessionId: m.ID})
	if err != nil || len(raw.Events) != 128 || raw.Events[2].Kind != "raw" {
		t.Fatal(raw, err)
	}
	for _, limit := range []uint32{1024, ^uint32(0)} {
		large, err := s.History(ctx, &api.WatchRequest{SessionId: m.ID, Limit: limit})
		if err != nil || len(large.Events) != 1024 || large.Events[1023].Seq != 1024 {
			t.Fatal("native history limit not honored or capped", large, err)
		}
	}
	details, err := s.EventDetails(ctx, &api.EventDetailsRequest{SessionId: m.ID, Seq: 2})
	if err != nil || len(details.Events) != 2 || details.Events[1].Text != "full result" {
		t.Fatal(details, err)
	}
}

// Opt-in replay of a private snapshot. No recorded transcript is checked in or
// printed; the test reports only transport sizes and display-record counts.
func TestTranscriptRecordedJournal(t *testing.T) {
	path := os.Getenv("CXZ_REPLAY_EVENTS")
	if path == "" {
		t.Skip("private journal replay")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var records []core.Event
	if err = json.Unmarshal(data, &records); err != nil {
		t.Fatal(err)
	}
	s, m, _ := projectionFixture(t)
	for i := range records {
		records[i].SessionID = m.ID
	}
	journalData, _ := json.Marshal(records)
	if len(records) > 0 && records[0].Seq > 1 {
		seq := records[0].Seq - 1
		payload, _ := json.Marshal(core.HistoryCheckpoint{Version: 1, Snapshot: core.Snapshot{LastSeq: seq}})
		checkpoint, _ := json.Marshal(core.Event{SessionID: m.ID, Seq: seq, Kind: core.HistoryCheckpointKind, Payload: payload})
		journalData = append(append(checkpoint, '\n'), journalData...)
	}
	if err = os.WriteFile(filepath.Join(core.Dir(s.root, m.ID), "events.jsonl"), append(journalData, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	page, err := s.Transcript(context.Background(), &api.TranscriptRequest{SessionId: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) == 0 || len(page.Events) > 256 {
		t.Fatal("invalid display window")
	}
	for _, e := range page.Events {
		if e.Kind == "raw" || e.Kind == "usage_status" {
			t.Fatal("protocol row exposed")
		}
		if e.ToolSummary != nil && len(e.Payload) > 0 {
			t.Fatal("historical tool body eagerly sent")
		}
	}
	wire, _ := json.Marshal(page)
	var window []core.Event
	for _, e := range records {
		if e.Seq >= page.Events[0].Seq {
			window = append(window, e)
		}
	}
	nativeWire, _ := json.Marshal(window)
	t.Logf("native records=%d; display rows=%d; summary bytes=%d; equivalent raw window records=%d; raw bytes=%d; legacy pages=%d", len(records), len(page.Events), len(wire), len(window), len(nativeWire), (len(window)+127)/128)
	older, err := s.Transcript(context.Background(), &api.TranscriptRequest{SessionId: m.ID, BeforeSeq: page.Events[0].Seq, SnapshotSeq: page.SnapshotSeq})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range older.Events {
		if e.Seq >= page.Events[0].Seq {
			t.Fatal("overlapping history page")
		}
	}
}
