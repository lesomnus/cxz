package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBackgroundSessionIndicator(t *testing.T) {
	m := conversationModel()
	s := m.current()
	m.observeSessions([]*api.Session{s})
	m.sessionActivity[s.Id].done = 10
	m.events[s.Id] = []*api.Event{bgEvent(1, `{"type":"system","subtype":"task_started","task_id":"bg","is_backgrounded":true}`)}
	first := ansi.Strip(m.sessionIndicator(s))
	if first != workingSpinner(0) {
		t.Fatal("idle foreground hid background work", first)
	}
	cached := m.backgroundStatesFor(s.Id)[s.RunId]
	m.Update(pulseTick{})
	if ansi.Strip(m.sessionIndicator(s)) != workingSpinner(1) || m.backgroundStatesFor(s.Id)[s.RunId] != cached {
		t.Fatal("spinner did not animate or replayed background telemetry")
	}
	m.events[s.Id] = append(m.events[s.Id], bgEvent(2, `{"type":"system","subtype":"task_notification","task_id":"bg","status":"completed"}`))
	if ansi.Strip(m.sessionIndicator(s)) != "+" {
		t.Fatal("completed background task left spinner or hid unread reply")
	}
	m.events[s.Id] = append(m.events[s.Id], bgEvent(3, `{"type":"system","subtype":"task_started","task_id":"another","is_backgrounded":true}`))
	if !m.hasActiveBackground(s) {
		t.Fatal("new background task not detected")
	}
	for _, state := range []string{"stopped", "failed", "interrupted"} {
		s.State = state
		if m.hasActiveBackground(s) {
			t.Fatal("dead agent retained active spinner", state)
		}
	}
	s.State, s.RunId = "idle", "replacement"
	if m.hasActiveBackground(s) {
		t.Fatal("old run leaked background work into resumed session")
	}
}

type sessionBackgroundClient struct {
	api.SessionsClient
	calls []string
	reply *api.BackgroundReply
	err   error
}

func (c *sessionBackgroundClient) Background(_ context.Context, r *api.SessionRef, _ ...grpc.CallOption) (*api.BackgroundReply, error) {
	c.calls = append(c.calls, r.Id)
	return c.reply, c.err
}

func backgroundReply(seq uint64, active bool) *api.BackgroundReply {
	data, _ := json.Marshal(map[string]*agentview.BackgroundState{"run": {Tasks: map[string]agentview.BackgroundTask{"bg": {ID: "bg", Active: active}}}})
	return &api.BackgroundReply{LastSeq: seq, Data: data}
}

func TestUnopenedSessionBackgroundRefresh(t *testing.T) {
	m := conversationModel()
	c := &sessionBackgroundClient{reply: backgroundReply(10, true)}
	m.client = c
	s := &api.Session{Id: "work::same", RunId: "run", State: "idle", LastSeq: 10}
	other := &api.Session{Id: "home::same", RunId: "run", State: "working", LastSeq: 10}
	m.allSessions = []*api.Session{s, other}
	cmd := m.refreshSessionBackground()
	if cmd == nil || len(c.calls) != 0 || m.refreshSessionBackground() != nil {
		t.Fatal("snapshot fetch blocked UI or scheduled duplicate requests")
	}
	m.Update(cmd())
	if len(c.calls) != 1 || c.calls[0] != s.Id || !m.hasActiveBackground(s) {
		t.Fatal("unopened remote session lost its background spinner", c.calls)
	}
	if len(m.events[s.Id]) != 0 || m.hasActiveBackground(other) || m.refreshSessionBackground() != nil {
		t.Fatal("loaded transcript, crossed connections, or refetched unchanged state")
	}
	// A sequence change while a snapshot is pending is picked up on completion.
	s.LastSeq = 11
	c.reply = backgroundReply(11, true)
	cmd = m.refreshSessionBackground()
	msg := cmd()
	s.LastSeq = 12
	c.reply = backgroundReply(12, false)
	_, next := m.Update(msg)
	if next == nil {
		t.Fatal("newer metadata was lost while request was in flight")
	}
	m.Update(next())
	if m.hasActiveBackground(s) || len(c.calls) != 3 {
		t.Fatal("completed background work still shows active", c.calls)
	}
	// Do not accept a response for a run replaced while the RPC was in flight.
	s.LastSeq = 13
	c.reply = backgroundReply(13, true)
	cmd = m.refreshSessionBackground()
	s.RunId, s.LastSeq = "replacement", 14
	m.Update(cmd())
	if m.backgroundSnapshots[s.Id].lastSeq != 12 || m.hasActiveBackground(s) {
		t.Fatal("stale request changed replacement run")
	}
}

func TestSessionBackgroundRefreshBoundedAndFailures(t *testing.T) {
	m := conversationModel()
	c := &sessionBackgroundClient{reply: backgroundReply(10, true)}
	m.client = c
	for i := range 7 {
		m.allSessions = append(m.allSessions, &api.Session{Id: fmt.Sprint(i), RunId: "run", State: "idle", LastSeq: 10})
	}
	batch := m.refreshSessionBackground()().(tea.BatchMsg)
	if len(batch) != 4 || m.refreshSessionBackground() != nil {
		t.Fatal("metadata fan-out is not bounded", len(batch))
	}
	queue := append([]tea.Cmd(nil), batch...)
	for len(queue) > 0 {
		cmd := queue[0]
		queue = queue[1:]
		_, next := m.Update(cmd())
		if next != nil {
			queue = append(queue, next)
		}
	}
	if len(c.calls) != 7 {
		t.Fatal("queued sessions were skipped", c.calls)
	}
	s := m.allSessions[0]
	s.LastSeq++
	c.err = status.Error(codes.Unimplemented, "old runtime")
	cmd := m.refreshSessionBackground()
	_, next := m.Update(cmd())
	if next != nil || m.refreshSessionBackground() != nil || !m.hasActiveBackground(s) {
		t.Fatal("failed metadata request retried immediately or erased known activity")
	}
}

func TestBackgroundSnapshotOrdering(t *testing.T) {
	m := conversationModel()
	m.storeBackgroundSnapshot("s", backgroundSnapshot{lastSeq: 20, states: map[string]*agentview.BackgroundState{"run": {Tasks: map[string]agentview.BackgroundTask{}}}})
	if m.hasActiveBackground(m.current()) {
		t.Fatal("empty snapshot has work")
	}
	m.Update(backgroundHistory{id: "s", snapshot: backgroundSnapshot{lastSeq: 10, states: map[string]*agentview.BackgroundState{"run": {Tasks: map[string]agentview.BackgroundTask{"bg": {Active: true}}}}}})
	if m.hasActiveBackground(m.current()) || m.backgroundSnapshots["s"].lastSeq != 20 {
		t.Fatal("late transcript snapshot resurrected completed background work")
	}
}

func TestSessionBackgroundRequestCancelsOnExit(t *testing.T) {
	m := conversationModel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.ctx, m.client = ctx, &backgroundClient{wait: true}
	m.allSessions = []*api.Session{m.current()}
	cmd := m.refreshSessionBackground()
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	cancel()
	select {
	case msg := <-done:
		if msg.(sessionBackgroundChecked).err == nil {
			t.Fatal("metadata request ignored TUI cancellation")
		}
		if _, next := m.Update(msg); next != nil {
			t.Fatal("shutting down TUI started another metadata request")
		}
	case <-time.After(time.Second):
		t.Fatal("metadata request survived TUI shutdown")
	}
}

func TestPanelSessionIndicatorSpacing(t *testing.T) {
	for _, width := range []int{60, 110, 200} {
		m := panelModel()
		s := m.current()
		s.Alias, s.State = "cedar", "working"
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		m.focusPanel()
		text := ansi.Strip(m.panelScreen())
		if !strings.Contains(text, "\n › ⣟ cedar · claude") {
			t.Fatal("session indentation or cursor gap is wrong", width, text)
		}
	}
}
