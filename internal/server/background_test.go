package server

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/eventwire"
	"github.com/lesomnus/cxz/internal/supervisor"
	"testing"
)

func TestBackgroundJournalProjection(t *testing.T) {
	raw := []byte(`{"type":"system","subtype":"background_tasks_changed","tasks":[]}`)
	e := core.Event{Kind: "raw", Raw: raw, Seq: 7, RunID: "run"}
	v := eventwire.FromCore(e)
	if v.Kind != "background" || string(v.Payload) != string(raw) || v.Seq != 7 || v.RunId != "run" {
		t.Fatal(v)
	}
	if e.Kind != "raw" || e.Payload != nil {
		t.Fatal("journal mutated")
	}
	if eventwire.FromCore(core.Event{Kind: "raw", Raw: []byte(`{"type":"system","subtype":"init"}`)}).Kind != "raw" {
		t.Fatal("init misclassified")
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
