package sessionstore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
)

func TestBackgroundOrderSurvivesStoreAndBackfillsLegacyRecords(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := Open(ctx, Path(dir), "s", true)
	if err != nil {
		t.Fatal(err)
	}
	raw := []string{
		`{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[{"task_id":"a","description":"first"}]}`,
		`{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[{"task_id":"b","description":"second"}]}`,
		`{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[]}`,
	}
	for index, payload := range raw {
		if err = store.Append(ctx, []core.Event{{SessionID: "s", RunID: "run", Seq: uint64(index + 1), TimeMS: int64((index + 1) * 1000), Kind: "background", Payload: json.RawMessage(payload)}}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := store.Background(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if data["run"].Tasks["a"].UpdatedSeq != 2 || data["run"].Tasks["b"].UpdatedSeq != 3 {
		t.Fatal(data)
	}
	encoded, _ := json.Marshal(data)
	if !strings.Contains(string(encoded), `"updated_seq":"3"`) {
		t.Fatal(string(encoded))
	}
	// Simulate the metadata written before sequence/time fields existed.
	for id, task := range data["run"].Tasks {
		task.UpdatedSeq, task.UpdatedAtMS = 0, 0
		data["run"].Tasks[id] = task
	}
	data["run"].Tasks["gone"] = agentview.BackgroundTask{ID: "gone", Status: "completed"}
	encoded, _ = json.Marshal(data)
	if _, err = store.DB().ExecContext(ctx, `UPDATE meta SET value=? WHERE key='background'`, string(encoded)); err != nil {
		t.Fatal(err)
	}
	store.Close()
	log, err := OpenLog(ctx, dir, "s")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	recovered, err := log.Store().Background(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if task := recovered["run"].Tasks["a"]; task.UpdatedSeq != 2 || task.UpdatedAtMS != 2000 {
		t.Fatal(task)
	}
	if task := recovered["run"].Tasks["b"]; task.UpdatedSeq != 3 || task.UpdatedAtMS != 3000 {
		t.Fatal(task)
	}
	if task := recovered["run"].Tasks["gone"]; task.ID != "gone" || task.UpdatedSeq != 0 {
		t.Fatal("invented ordering or deleted an unrecoverable task", task)
	}
	// Migration is idempotent and never invents a date from restart time.
	if err = log.Store().backfillBackgroundOrder(ctx, nil); err != nil {
		t.Fatal(err)
	}
	again, _ := log.Store().Background(ctx)
	if again["run"].Tasks["b"].UpdatedSeq != 3 {
		t.Fatal(again)
	}
}
