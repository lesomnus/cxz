package tui

import (
	"bytes"
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/notification"
)

func expectSound(t *testing.T, cmd tea.Cmd, sound notification.Sound) {
	t.Helper()
	if cmd == nil {
		t.Fatal("missing alert")
	}
	msg, ok := cmd().(soundRequested)
	if !ok || msg.sound != sound {
		t.Fatalf("unexpected alert: %#v", msg)
	}
}

func TestSessionSounds(t *testing.T) {
	m := conversationModel()
	s := &api.Session{Id: "remote::session", RunId: "run", State: "waiting_input", LastSeq: 10, Pending: []*api.Event{{RunId: "run", RequestId: "old", Seq: 9, Text: "AskUserQuestion"}}}
	if m.observeSessions([]*api.Session{s}) != nil {
		t.Fatal("startup played historical alert")
	}
	s.State, s.LastSeq = "working", 11
	s.Pending = nil
	m.observeSessions([]*api.Session{s})
	s.State, s.LastSeq = "waiting_input", 12
	s.Pending = []*api.Event{{RunId: "run", RequestId: "new", Seq: 12, Text: "AskUserQuestion"}}
	expectSound(t, m.observeSessions([]*api.Session{s}), notification.Attention)
	if m.observeSessions([]*api.Session{s}) != nil {
		t.Fatal("duplicate pending alert")
	}
	s.State, s.LastSeq = "idle", 13
	s.Pending = nil
	m.sessionActivity[s.Id].seen = 13 // Reading the reply does not suppress its sound.
	expectSound(t, m.observeSessions([]*api.Session{s}), notification.Complete)
	if m.observeSessions([]*api.Session{s}) != nil {
		t.Fatal("duplicate completion alert")
	}
	s.LastSeq = 12
	s.Pending = []*api.Event{{RunId: "run", RequestId: "new", Seq: 12, Text: "AskUserQuestion"}}
	if m.observeSessions([]*api.Session{s}) != nil {
		t.Fatal("stale snapshot played an alert")
	}
}

func TestPendingSoundIgnoresFocusAndAutomaticApprovals(t *testing.T) {
	m := conversationModel()
	m.projectView = true
	s := &api.Session{Id: "s", RunId: "r", State: "working", PermissionMode: "full"}
	m.observeSessions([]*api.Session{s})
	s.LastSeq = 1
	s.Pending = []*api.Event{{RunId: "r", RequestId: "auto", Seq: 1, Text: "Bash"}}
	if !automaticApproval(s.Pending[0]) {
		t.Fatal("fixture is not an automatic approval")
	}
	if m.observeSessions([]*api.Session{s}) != nil {
		t.Fatal("automatic permission alerted")
	}
	s.LastSeq = 2
	s.Pending = append(s.Pending, &api.Event{RunId: "r", RequestId: "question", Seq: 2, Text: "AskUserQuestion"})
	expectSound(t, m.observeSessions([]*api.Session{s}), notification.Attention)
}

func TestCompletionSoundBetweenSnapshots(t *testing.T) {
	m := conversationModel()
	client := &completionClient{events: []*api.Event{{Seq: 20, RunId: "r", Kind: "turn_end"}}}
	m.client = client
	s := &api.Session{Id: "s", RunId: "r", State: "idle", LastSeq: 10}
	m.observeSessions([]*api.Session{s})
	for _, seq := range []uint64{20, 30} {
		client.events = append(client.events, &api.Event{Seq: seq, RunId: "r", Kind: "turn_end"})
		s.LastSeq = seq
		cmd := m.observeSessions([]*api.Session{s})
		if cmd == nil {
			t.Fatal("no completion probe")
		}
		result := cmd().(completionChecked)
		expectSound(t, m.receiveCompletion(result), notification.Complete)
		if m.receiveCompletion(result) != nil {
			t.Fatal("probe replay alerted")
		}
	}
}

func TestSSHBellAndCancelledNotification(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "remote")
	var output bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &model{ctx: ctx, cursorOutput: &cursorWriter{out: &output}}
	m.playNotification(notification.Complete)()
	if output.String() != "\a" {
		t.Fatalf("bell output %q", output.String())
	}
	cancel()
	m.playNotification(notification.Attention)()
	if output.String() != "\a" {
		t.Fatal("cancelled notification rang bell")
	}
}

func TestNewRunAlertsAndStaleProbe(t *testing.T) {
	m := conversationModel()
	m.client = &completionClient{events: []*api.Event{{Seq: 12, RunId: "new", Kind: "turn_end"}}}
	s := &api.Session{Id: "s", RunId: "old", State: "idle", LastSeq: 10}
	m.observeSessions([]*api.Session{s})
	s.RunId, s.LastSeq = "new", 12
	cmd := m.observeSessions([]*api.Session{s})
	if cmd == nil {
		t.Fatal("short new run was missed")
	}
	result := cmd().(completionChecked)
	expectSound(t, m.receiveCompletion(result), notification.Complete)
	s.RunId, s.State, s.LastSeq = "newer", "waiting_input", 14
	s.Pending = []*api.Event{{RunId: "newer", RequestId: "q", Seq: 14, Text: "AskUserQuestion"}}
	expectSound(t, m.observeSessions([]*api.Session{s}), notification.Attention)
	if m.receiveCompletion(result) != nil {
		t.Fatal("old run probe played a sound")
	}
}
