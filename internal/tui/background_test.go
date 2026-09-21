package tui

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/agentview"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

func bgEvent(seq uint64, raw string) *api.Event {
	return &api.Event{Seq: seq, RunId: "run", Kind: "background", Payload: []byte(raw)}
}

func TestBackgroundLaunchIsNotCompletion(t *testing.T) {
	m := conversationModel()
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "run", Kind: "tool_call", RequestId: "tool", Text: "Bash", Payload: []byte(`{"command":"sleep 300"}`)},
		bgEvent(2, `{"type":"system","subtype":"task_started","task_id":"bg","tool_use_id":"tool","is_backgrounded":true}`),
		{Seq: 3, RunId: "run", Kind: "tool_result", RequestId: "tool", Payload: []byte(`{"content":"Command running in background"}`)},
	}
	m.render()
	if text := ansi.Strip(m.view.View()); strings.Count(text, "Bash") != 1 || !strings.Contains(text, "[•] Bash") {
		t.Fatal(text)
	}
	if !strings.Contains(m.backgroundStatus(), "background 1") || m.activeWork() {
		t.Fatal("background must remain visible independently of idle foreground")
	}
	m.events["s"] = append(m.events["s"], bgEvent(4, `{"type":"system","subtype":"background_tasks_changed","tasks":[]}`))
	m.render()
	if m.backgroundStatus() != "" || strings.Contains(ansi.Strip(m.view.View()), "[✓] Bash") {
		t.Fatal("empty snapshot fabricated completion")
	}
	m.events["s"] = append(m.events["s"], bgEvent(5, `{"type":"system","subtype":"task_notification","task_id":"bg","status":"failed","summary":"exit 1","output_file":"/not/read"}`))
	m.render()
	if text := ansi.Strip(m.view.View()); !strings.Contains(text, "[×] Bash") || strings.Count(text, "Bash") != 1 {
		t.Fatal(text)
	}
	if !strings.Contains(m.backgroundReport(), "/not/read") {
		t.Fatal(m.backgroundReport())
	}
	m.current().RunId = "new"
	if strings.Contains(m.backgroundReport(), "exit 1") {
		t.Fatal("cross-run leak")
	}
}

type backgroundClient struct {
	api.SessionsClient
	histories, snapshots int
	err                  error
	wait                 bool
}

func (c *backgroundClient) History(context.Context, *api.WatchRequest, ...grpc.CallOption) (*api.EventBatch, error) {
	c.histories++
	return nil, status.Error(codes.Internal, "history must not be fetched")
}
func (c *backgroundClient) Background(ctx context.Context, _ *api.SessionRef, _ ...grpc.CallOption) (*api.BackgroundReply, error) {
	c.snapshots++
	if c.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if c.err != nil {
		return nil, c.err
	}
	states := map[string]*agentview.BackgroundState{"run": {Tasks: map[string]agentview.BackgroundTask{"bg": {ID: "bg", Active: true, Status: "working"}}}}
	data, _ := json.Marshal(states)
	return &api.BackgroundReply{LastSeq: 300, Data: data}, nil
}
func TestBackgroundBackfillDoesNotLoadTranscript(t *testing.T) {
	m := conversationModel()
	m.ctx = context.Background()
	client := &backgroundClient{}
	m.client = client
	m.events["s"] = []*api.Event{{Seq: 300, RunId: "run", Kind: "assistant", Text: "tail"}}
	cmd := m.loadBackgroundHistory(historyPage{id: "s", initial: true, start: 256})
	m.Update(cmd())
	if client.histories != 0 || client.snapshots != 1 {
		t.Fatal("whole history downloaded", client)
	}
	if len(m.events["s"]) != 1 || !strings.Contains(m.backgroundStatus(), "background 1") {
		t.Fatal("metadata snapshot failed")
	}
	m.events["s"] = append(m.events["s"], bgEvent(301, `{"type":"system","subtype":"background_tasks_changed","tasks":[]}`))
	if m.backgroundStatus() != "" {
		t.Fatal("old metadata overrode latest snapshot")
	}
	m.watchEpoch = 2
	if m.loadBackgroundHistory(historyPage{id: "s", initial: true, start: 256, epoch: 1}) != nil {
		t.Fatal("stale backfill")
	}
}

func TestBackgroundSnapshotDoesNotRewindAndOlderServerDoesNotScan(t *testing.T) {
	m := conversationModel()
	client := &backgroundClient{}
	m.client = client
	m.ctx = context.Background()
	m.events["s"] = []*api.Event{bgEvent(20, `{"type":"system","subtype":"background_tasks_changed","tasks":[]}`)}
	m.Update(m.loadBackgroundHistory(historyPage{id: "s", initial: true, start: 100000})())
	if !strings.Contains(m.backgroundStatus(), "background 1") {
		t.Fatal("old page rewound snapshot")
	}
	m.events["s"] = append(m.events["s"], bgEvent(301, `{"type":"system","subtype":"task_notification","task_id":"bg","status":"completed"}`))
	if m.backgroundStatus() != "" {
		t.Fatal("new live event did not advance snapshot")
	}
	if !m.backgroundSnapshots["s"].states["run"].Tasks["bg"].Active {
		t.Fatal("render mutated snapshot")
	}
	client.err = status.Error(codes.Unimplemented, "old runtime")
	m.Update(m.loadBackgroundHistory(historyPage{id: "s", initial: true, start: 100000})())
	if client.histories != 0 || !strings.Contains(m.backgroundErrors["s"], "update") {
		t.Fatal("old server triggered replay", client.histories)
	}
}

func TestBackgroundSnapshotCancelsWithConversation(t *testing.T) {
	m := conversationModel()
	m.client = &backgroundClient{wait: true}
	ctx, cancel := context.WithCancel(context.Background())
	m.watchContext = ctx
	cmd := m.loadBackgroundHistory(historyPage{id: "s", initial: true, start: 100000})
	cancel()
	done := make(chan backgroundHistory, 1)
	go func() { done <- cmd().(backgroundHistory) }()
	select {
	case result := <-done:
		if result.err != context.Canceled {
			t.Fatal(result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("background request survived session switch")
	}
}
