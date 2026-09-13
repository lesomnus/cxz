package tui

import (
	"context"
	"strings"
	"testing"

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

type backgroundClient struct{ api.SessionsClient }

func (c *backgroundClient) History(_ context.Context, r *api.WatchRequest, _ ...grpc.CallOption) (*api.EventBatch, error) {
	b := &api.EventBatch{}
	for seq := r.AfterSeq + 1; seq <= r.AfterSeq+128; seq++ {
		e := &api.Event{Seq: seq, RunId: "run", Kind: "raw"}
		if seq == 1 {
			e = bgEvent(seq, `{"type":"system","subtype":"task_started","task_id":"bg","is_backgrounded":true}`)
		}
		b.Events = append(b.Events, e)
	}
	return b, nil
}
func TestBackgroundBackfillDoesNotLoadTranscript(t *testing.T) {
	m := conversationModel()
	m.ctx = context.Background()
	m.client = &backgroundClient{}
	m.events["s"] = []*api.Event{{Seq: 300, RunId: "run", Kind: "assistant", Text: "tail"}}
	cmd := m.loadBackgroundHistory(historyPage{id: "s", initial: true, start: 256})
	m.Update(cmd())
	if len(m.events["s"]) != 1 || !strings.Contains(m.backgroundStatus(), "background 1") {
		t.Fatal("metadata backfill failed")
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
