package server

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/eventview"
	"github.com/lesomnus/cxz/internal/supervisor"
	"testing"
)

func TestBackgroundJournalProjection(t *testing.T) {
	raw := []byte(`{"type":"system","subtype":"background_tasks_changed","tasks":[]}`)
	e := core.Event{Kind: "raw", Raw: raw, Seq: 7, RunID: "run"}
	v := eventview.FromCore(e)
	if v.Kind != "background" || string(v.Payload) != string(raw) || v.Seq != 7 || v.RunId != "run" {
		t.Fatal(v)
	}
	if e.Kind != "raw" || e.Payload != nil {
		t.Fatal("journal mutated")
	}
	if eventview.FromCore(core.Event{Kind: "raw", Raw: []byte(`{"type":"system","subtype":"init"}`)}).Kind != "raw" {
		t.Fatal("init misclassified")
	}
}

func TestBackgroundTelemetryUpdatesStateWithoutConversationRows(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			s, m, log := projectionFixture(t)
			ctx := context.Background()
			if _, err := log.Append(core.Event{SessionID: m.ID, RunID: "run", Kind: "assistant", Text: "Running the server."}); err != nil {
				t.Fatal(err)
			}
			for _, active := range []bool{true, false} {
				tasks := []agentview.BackgroundTask{}
				if active {
					tasks = append(tasks, agentview.BackgroundTask{ID: "bg", Description: "server", Active: true})
				}
				kind := "system"
				if provider == "codex" {
					kind = "background"
				}
				payload, err := json.Marshal(map[string]any{"type": kind, "provider": provider, "subtype": "background_tasks_changed", "tasks": tasks})
				if err != nil {
					t.Fatal(err)
				}
				e := core.Event{SessionID: m.ID, RunID: "run", Kind: "background", Text: "background_tasks_changed", Payload: payload}
				if provider == "claude" {
					e.Kind, e.Raw, e.Payload = "raw", payload, nil
				}
				written, err := log.Append(e)
				if err != nil {
					t.Fatal(err)
				}
				background, err := s.Background(ctx, &api.SessionRef{Id: m.ID})
				if err != nil {
					t.Fatal(err)
				}
				var states map[string]agentview.BackgroundState
				if err = json.Unmarshal(background.Data, &states); err != nil || states["run"].Tasks["bg"].Active != active {
					t.Fatal("background state stopped updating", states, err)
				}
				page, err := s.Transcript(ctx, &api.TranscriptRequest{SessionId: m.ID})
				if err != nil || len(page.Events) != 1 || page.Events[0].Text != "Running the server." || page.SnapshotSeq != written.Seq {
					t.Fatal("telemetry became a conversation card", page, err)
				}
				rows, err := s.syncedRows(ctx, m, 1)
				if err != nil || len(rows) != 0 {
					t.Fatal("telemetry became a live row", rows, err)
				}
				details, err := s.EventDetails(ctx, &api.EventDetailsRequest{SessionId: m.ID, Seq: written.Seq})
				if err != nil || len(details.Events) != 1 || string(details.Events[0].Payload) != string(payload) {
					t.Fatal("the telemetry record was lost", details, err)
				}
			}
		})
	}
}

func TestCodexBackgroundSnapshotProjection(t *testing.T) {
	s, m, log := projectionFixture(t)
	for _, payload := range []string{
		`{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[{"task_id":"42","tool_use_id":"tool","description":"server","active":true}]}`,
		`{"type":"background","provider":"codex","subtype":"background_status","status":"error"}`,
		`{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[]}`,
	} {
		if _, err := log.Append(core.Event{SessionID: m.ID, RunID: "codex-run", Kind: "background", Payload: []byte(payload)}); err != nil {
			t.Fatal(err)
		}
		reply, err := s.Background(context.Background(), &api.SessionRef{Id: m.ID})
		if err != nil {
			t.Fatal(err)
		}
		var states map[string]agentview.BackgroundState
		if err = json.Unmarshal(reply.Data, &states); err != nil {
			t.Fatal(err)
		}
		state := states["codex-run"]
		if agentview.IsBackgroundEvent([]byte(payload)) != true {
			t.Fatal("normalized envelope rejected")
		}
		if state.Unavailable != (reply.LastSeq == 2) {
			t.Fatal("availability lost", string(reply.Data))
		}
		if reply.LastSeq == 1 && !state.Tasks["42"].Active {
			t.Fatal("missing active terminal", state)
		}
		if reply.LastSeq == 3 && (state.Tasks["42"].Active || !state.Snapshot) {
			t.Fatal("empty list did not clear active flag", state)
		}
		// Restarting the projection reconstructs the same availability and list.
		restarted := &Server{root: s.root, db: s.db}
		recovered, err := restarted.Background(context.Background(), &api.SessionRef{Id: m.ID})
		if err != nil || string(recovered.Data) != string(reply.Data) {
			t.Fatal(recovered, err)
		}
	}
	checkpoint := core.HistoryCheckpoint{
		Version:  1,
		Snapshot: supervisor.Replay(log.All()),
		Resume:   []core.Event{log.All()[len(log.All())-1]},
	}
	encoded, _ := json.Marshal(checkpoint)
	last := log.All()[len(log.All())-1]
	last.Kind = core.HistoryCheckpointKind
	last.Payload = encoded
	if err := log.Compact(last); err != nil {
		t.Fatal(err)
	}
	restarted := &Server{root: s.root, db: s.db}
	reply, err := restarted.Background(context.Background(), &api.SessionRef{Id: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	var states map[string]agentview.BackgroundState
	if err = json.Unmarshal(reply.Data, &states); err != nil {
		t.Fatal(err)
	}
	if !states["codex-run"].Snapshot || states["codex-run"].Unavailable {
		t.Fatal("empty list lost across retention", string(reply.Data))
	}
}
