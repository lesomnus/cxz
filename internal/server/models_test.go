package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

// The catalog is served from the projection, which is the whole point: a client
// asking what the agent can run must not pay for the conversation. The journal
// here is long enough that a scan would need several pages; the reply arrives in
// one request and names the newest record of the current run.
func TestModelsAnswersFromTheProjection(t *testing.T) {
	s, m, l := projectionFixture(t)
	ctx := context.Background()
	first, _ := json.Marshal(map[string]any{"model": "old", "models": []map[string]any{{"id": "old"}}})
	latest, _ := json.Marshal(map[string]any{"model": "new", "models": []map[string]any{{"id": "new"}}, "applied_reported": true})
	other, _ := json.Marshal(map[string]any{"model": "elsewhere"})
	batch := []core.Event{
		{Kind: "state", Text: "idle", RunID: "run"},
		{Kind: "models", Text: "catalog", RunID: "run", Payload: first},
		{Kind: "models", Text: "catalog", RunID: "previous", Payload: other},
	}
	for i := 0; i < 400; i++ {
		batch = append(batch, core.Event{Kind: "assistant", RunID: "run", Text: "history"})
	}
	batch = append(batch, core.Event{Kind: "models", Text: "catalog", RunID: "run", Payload: latest})
	for i := range batch {
		batch[i].SessionID = m.ID
	}
	if _, err := l.AppendBatch(batch); err != nil {
		t.Fatal(err)
	}
	reply, err := s.Models(ctx, &api.ModelsRequest{SessionId: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	if reply.RunId != "run" {
		t.Fatal("the reply describes another run:", reply.RunId)
	}
	var got struct {
		Model           string `json:"model"`
		AppliedReported bool   `json:"applied_reported"`
	}
	if err = json.Unmarshal(reply.Data, &got); err != nil {
		t.Fatal(err, string(reply.Data))
	}
	// Last one wins: a run republishes after a setting is applied.
	if got.Model != "new" || !got.AppliedReported {
		t.Fatalf("%+v", got)
	}
	if reply.CatalogSeq != uint64(len(batch)) || reply.CatalogMs == 0 {
		t.Fatalf("the record is not located in the journal: seq=%d ms=%d", reply.CatalogSeq, reply.CatalogMs)
	}
	if reply.LastSeq != uint64(len(batch)) {
		t.Fatal("cursor not reported:", reply.LastSeq)
	}
	if reply.Refreshing {
		t.Fatal("a read asked the provider")
	}
}

// A run with no record is a state to report, not an error: the picker says so
// and offers to ask the provider.
func TestModelsWithoutARecord(t *testing.T) {
	s, m, l := projectionFixture(t)
	other, _ := json.Marshal(map[string]any{"model": "elsewhere"})
	for _, e := range []core.Event{
		{Kind: "state", Text: "idle", RunID: "run", SessionID: m.ID},
		{Kind: "models", Text: "catalog", RunID: "previous", Payload: other, SessionID: m.ID},
	} {
		if _, err := l.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	reply, err := s.Models(context.Background(), &api.ModelsRequest{SessionId: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Data) != 0 || reply.CatalogSeq != 0 {
		t.Fatal("a record from another run was served:", string(reply.Data))
	}
	if reply.RunId != "run" || reply.LastSeq != 2 {
		t.Fatalf("%+v", reply)
	}
}

// Refresh reaches the supervisor. Without one running, the reason belongs beside
// the catalog rather than replacing it with an error: what is already known is
// still the best answer.
func TestModelsRefreshReportsItsReasonAndStillAnswers(t *testing.T) {
	s, m, l := projectionFixture(t)
	payload, _ := json.Marshal(map[string]any{"model": "known", "models": []map[string]any{{"id": "known"}}})
	for _, e := range []core.Event{
		{Kind: "state", Text: "idle", RunID: "run", SessionID: m.ID},
		{Kind: "models", Text: "catalog", RunID: "run", Payload: payload, SessionID: m.ID},
	} {
		if _, err := l.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	reply, err := s.Models(context.Background(), &api.ModelsRequest{SessionId: m.ID, Refresh: true})
	if err != nil {
		t.Fatal("a refusal to refresh became a failure to report:", err)
	}
	if reply.Refreshing {
		t.Fatal("reported asking a supervisor that is not running")
	}
	if reply.Status == "" {
		t.Fatal("no reason given for not asking")
	}
	if len(reply.Data) == 0 {
		t.Fatal("the known catalog was dropped")
	}
}
