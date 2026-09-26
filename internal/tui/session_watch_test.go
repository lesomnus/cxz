package tui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type multipleWatchClient struct {
	api.SessionsClient
	streams  map[string]chan streamEvent
	requests chan *api.WatchRequest
}

func (c *multipleWatchClient) Watch(ctx context.Context, r *api.WatchRequest, _ ...grpc.CallOption) (api.Sessions_WatchClient, error) {
	c.requests <- r
	return &liveWatchStream{channelEvents: channelEvents{ctx, c.streams[r.SessionId]}}, nil
}

func TestUnselectedSessionsStayCurrentWithoutReplayingOnSwitch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	m := conversationModel()
	m.ctx = ctx
	m.sessions = append(m.sessions, &api.Session{Id: "second", Agent: "claude", RunId: "other-run"})
	m.allSessions = m.sessions
	c := &multipleWatchClient{streams: map[string]chan streamEvent{"s": make(chan streamEvent, 2), "second": make(chan streamEvent, 2)}, requests: make(chan *api.WatchRequest, 4)}
	m.client = c
	probe := &liveWatchProbe{messages: make(chan caughtUp, 4)}
	p := tea.NewProgram(probe, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	m.program = p
	done := make(chan struct{})
	go func() { p.Run(); close(done) }()
	defer func() { cancel(); <-done }()
	m.watch()
	for range 2 {
		select {
		case <-c.requests:
		case <-ctx.Done():
			t.Fatal("not all sessions subscribed")
		}
	}
	before := m.view.View()
	c.streams["second"] <- streamEvent{event: &api.Event{Seq: 1, RunId: "other-run", Kind: "assistant", Text: "Already current when opened"}}
	select {
	case batch := <-probe.messages:
		m.Update(batch)
	case <-ctx.Done():
		t.Fatal("background conversation not delivered")
	}
	if m.cursor["second"] != 1 || m.view.View() != before {
		t.Fatal("background update did not stay in its own cache")
	}
	m.selected = 1
	m.watch()
	m.render()
	if !strings.Contains(m.view.View(), "Already current when opened") {
		t.Fatal("selection did not render the current cache")
	}
	select {
	case <-c.requests:
		t.Fatal("switch restarted a subscription")
	default:
	}
	m.project = &api.Project{Id: "p"}
	m.backToProject()
	if m.sessionWatches["second"].ctx.Err() != nil {
		t.Fatal("project navigation canceled the conversation stream")
	}
	old := m.sessionWatches["second"]
	m.sessions = m.sessions[:1]
	m.allSessions = m.sessions
	m.selected = 0
	m.watch()
	if old.ctx.Err() == nil {
		t.Fatal("removed session's subscription leaked")
	}
	m.Update(caughtUp{id: "second", epoch: old.epoch, events: []*api.Event{{Seq: 2, Kind: "assistant", Text: "stale"}}})
	if m.cursor["second"] != 1 {
		t.Fatal("removed stream's queued response was accepted")
	}
}
