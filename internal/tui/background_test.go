package tui

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/notification"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
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
	if text := ansi.Strip(m.view.View()); strings.Count(text, "Bash") != 1 || !strings.Contains(text, "• Bash") {
		t.Fatal(text)
	}
	if !strings.Contains(m.backgroundStatus(), "background 1") || m.activeWork() {
		t.Fatal("background must remain visible independently of idle foreground")
	}
	seen := map[string]bool{}
	for _, pulse := range []int{0, 1, 8} {
		m.pulse = pulse
		text := ansi.Strip(m.backgroundStatus())
		frame := spinnerFrame(text)
		if frame == "" || !strings.HasPrefix(text, frame+" background 1") {
			t.Fatal("background status does not use the shared eight-dot spinner")
		}
		seen[frame] = true
	}
	// Eight frames, so pulse 0 and 8 are the same one and pulse 1 is not: the
	// phase offsets the spinner without changing the rate it turns.
	if len(seen) != 2 {
		t.Fatal("spinner did not turn with the pulse", seen)
	}
	m.events["s"] = append(m.events["s"], bgEvent(4, `{"type":"system","subtype":"background_tasks_changed","tasks":[]}`))
	m.render()
	if m.backgroundStatus() != "" || strings.Contains(ansi.Strip(m.view.View()), "✓ Bash") {
		t.Fatal("empty snapshot fabricated completion")
	}
	m.events["s"] = append(m.events["s"], bgEvent(5, `{"type":"system","subtype":"task_notification","task_id":"bg","status":"failed","summary":"exit 1","output_file":"/not/read"}`))
	m.render()
	if text := ansi.Strip(m.view.View()); !strings.Contains(text, "× Bash") || strings.Count(text, "Bash") != 1 {
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

// Going idle by handing work to a background task is not a finished turn: the
// row keeps its spinner, so the completion sound must not contradict it.
func TestBackgroundLaunchIsSilentUntilTasksEnd(t *testing.T) {
	idle := func(m *model, events ...*api.Event) (*model, *api.Session) {
		s := m.current()
		s.State, s.LastSeq = "working", 1
		m.observeSessions([]*api.Session{s})
		m.events[s.Id] = events
		s.State, s.LastSeq = "idle", 2
		return m, s
	}
	// The same transition without background work still rings.
	m, _ := idle(conversationModel())
	expectSound(t, m.observeSessions([]*api.Session{m.current()}), notification.Complete)

	m, s := idle(conversationModel(), bgEvent(2, `{"type":"system","subtype":"task_started","task_id":"bg","tool_use_id":"tool","is_backgrounded":true}`))
	if m.observeSessions([]*api.Session{s}) != nil {
		t.Fatal("launching a background task rang the completion sound")
	}
	if ansi.Strip(m.sessionIndicator(s)) == "+" {
		t.Fatal("running background work marked the reply finished")
	}
	m.events[s.Id] = append(m.events[s.Id], bgEvent(3, `{"type":"system","subtype":"background_tasks_changed","tasks":[]}`))
	if ansi.Strip(m.sessionIndicator(s)) != "+" {
		t.Fatal("finished background work left no unread marker")
	}
}

// The spinner a background task leaves behind says something is running, not
// that the agent has the turn. The two greens are how the session column ranks
// those, so this one takes the quiet step and the bright one keeps meaning that
// a session is working.
func TestBackgroundOnlySpinnerTakesTheQuietStep(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	bright, quiet := sgr(running), sgr(accent)
	if bright == quiet {
		t.Fatal("the two greens render the same")
	}

	m := conversationModel()
	s := m.current()
	s.State = "idle"
	m.events[s.Id] = []*api.Event{bgEvent(2, `{"type":"system","subtype":"task_started","task_id":"bg","tool_use_id":"tool","is_backgrounded":true}`)}
	row := m.sessionIndicator(s)
	if got, want := ansi.Strip(row), workingSpinner(m.pulse+spinnerPhase(s.CreatedAt)); got != want {
		t.Fatal("background work left the row without its spinner:", got)
	}
	if !strings.Contains(row, quiet) || strings.Contains(row, bright) {
		t.Fatalf("a background-only spinner is not on the quiet step: %q", row)
	}

	// An agent working outranks whatever its background tasks are doing.
	s.State = "working"
	if row = m.sessionIndicator(s); !strings.Contains(row, bright) {
		t.Fatalf("a working session lost the focus step: %q", row)
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
