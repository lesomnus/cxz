package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/supervisor"
	"os"
	"testing"
	"time"
)

func TestProjectionPrunesCheckpointAndResumesSuffix(t *testing.T) {
	s, m, l := projectionFixture(t)
	for i := range 12 {
		kind, text := "assistant", "old"
		if i == 0 {
			kind, text = "vendor", "vendor-id"
		}
		if i == 6 {
			kind, text = "state", "idle"
		}
		l.Append(core.Event{SessionID: m.ID, RunID: "run", Kind: kind, Text: text})
	}
	p, err := s.lockProjection(t.Context(), m)
	if err != nil {
		t.Fatal(err)
	}
	p.mu.Unlock()
	before := l.All()
	snap := supervisor.Replay(before[:7])
	payload, _ := json.Marshal(core.HistoryCheckpoint{Version: 1, Snapshot: snap})
	if err = l.Compact(core.Event{SessionID: m.ID, Seq: 7, Kind: core.HistoryCheckpointKind, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	p, err = s.lockProjection(t.Context(), m)
	if err != nil {
		t.Fatal(err)
	}
	if p.snapshot.VendorID != "vendor-id" || p.snapshot.LastSeq != 12 {
		t.Fatal(p.snapshot)
	}
	p.mu.Unlock()
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if count != 6 {
		t.Fatal("old projection retained", count)
	}
	batch, err := s.History(t.Context(), &api.WatchRequest{SessionId: m.ID})
	if err != nil || len(batch.Events) != 6 || batch.Events[0].Kind != core.HistoryTrimmedKind || core.HistoryFloor(batch.Events[0].Kind, batch.Events[0].Payload) != 7 {
		t.Fatal(batch, err)
	}
	if string(batch.Events[0].Payload) == string(payload) {
		t.Fatal("resume metadata exposed")
	}
	l.Append(core.Event{SessionID: m.ID, Kind: "assistant", Text: "new"})
	batch, err = s.History(t.Context(), &api.WatchRequest{SessionId: m.ID, AfterSeq: 12})
	if err != nil || len(batch.Events) != 1 || batch.Events[0].Seq != 13 {
		t.Fatal(batch, err)
	}
	s.projections = nil
	p, err = s.lockProjection(t.Context(), m)
	if err != nil {
		t.Fatal(err)
	}
	p.mu.Unlock()
	var version int
	s.db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 2 {
		t.Fatal("legacy rollback not fenced", version)
	}
}

func TestRetentionPolicyAndSchemaFenceThroughPublicRPC(t *testing.T) {
	t.Setenv("CXZ_PROJECT_ID", "")
	t.Setenv("CXZ_OWNER", "")
	root, err := os.MkdirTemp("", "cxz-retention-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	for iteration := range 2 {
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- Run(ctx, root, "unused", "") }()
		conn, err := Dial(root)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		client := resourceclient.New(conn)
		deadline := time.Now().Add(10 * time.Second)
		var reply *api.HistoryPolicy
		for time.Now().Before(deadline) {
			q, stop := context.WithTimeout(t.Context(), time.Second)
			reply, err = client.GetHistoryPolicy(q, &api.Empty{})
			stop()
			if err == nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if err == nil && iteration == 0 {
			_, err = client.SetHistoryPolicy(t.Context(), &api.HistoryPolicy{MaxMib: 25})
			if err == nil {
				// Marking the projection is what the supervisor does before it
				// compacts, and what fences an older release off the file.
				_, err = client.MarkHistoryTrimmable(t.Context(), &api.Empty{})
			}
		} else if err == nil && reply.MaxMib != 25 {
			err = fmt.Errorf("policy lost across schema-2 restart: %d MiB", reply.MaxMib)
		}
		conn.Close()
		cancel()
		runErr := <-done
		if err != nil || runErr != nil {
			t.Fatal(err, runErr)
		}
	}
}
