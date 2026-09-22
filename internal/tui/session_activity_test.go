package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

func TestSessionIndicatorsAndReadAcknowledgement(t *testing.T) {
	m := conversationModel()
	s := m.current()
	s.LastSeq = 1
	m.observeSessions([]*api.Session{s})
	if m.sessionIndicator(s) != " " {
		t.Fatal("initial idle history marked unread")
	}
	s.State, s.LastSeq = "working", 2
	m.observeSessions([]*api.Session{s})
	first := m.sessionIndicator(s)
	m.Update(pulseTick{})
	if !strings.Contains(first, "⣟") || m.sessionIndicator(s) == first {
		t.Fatal("working spinner did not animate")
	}
	s.State, s.LastSeq = "waiting_input", 3
	m.observeSessions([]*api.Session{s})
	if m.sessionActivity[s.Id].done != 0 {
		t.Fatal("permission wait completed the turn")
	}
	m.focusPanel()
	s.State, s.LastSeq = "idle", 4
	m.observeSessions([]*api.Session{s})
	if ansi.Strip(m.sessionIndicator(s)) != "+" {
		t.Fatal("background completion not marked")
	}
	m.cursor[s.Id] = 4
	m.View()
	if ansi.Strip(m.sessionIndicator(s)) != "+" {
		t.Fatal("looking at project panel acknowledged completion")
	}
	m.panelFocus = false
	m.historyOpening = map[string]bool{s.Id: true}
	m.View()
	if ansi.Strip(m.sessionIndicator(s)) != "+" {
		t.Fatal("loading acknowledged unread content")
	}
	delete(m.historyOpening, s.Id)
	m.events[s.Id] = []*api.Event{{Seq: 4, Kind: "assistant", Text: strings.Repeat("response\n", 60)}}
	m.render()
	m.view.GotoTop()
	m.View()
	if ansi.Strip(m.sessionIndicator(s)) != "+" {
		t.Fatal("older viewport acknowledged completion")
	}
	m.view.GotoBottom()
	m.filePreview = &filePreview{session: s.Id, title: "Read", source: "code", focused: true}
	m.View()
	if ansi.Strip(m.sessionIndicator(s)) != "+" {
		t.Fatal("focused tool preview acknowledged completion")
	}
	m.filePreview = nil
	m.View()
	if m.sessionIndicator(s) != " " {
		t.Fatal("viewing latest response left unread marker")
	}
}

type completionClient struct {
	api.SessionsClient
	events []*api.Event
	calls  int
}

func (c *completionClient) History(_ context.Context, req *api.WatchRequest, _ ...grpc.CallOption) (*api.EventBatch, error) {
	c.calls++
	out := &api.EventBatch{}
	for _, e := range c.events {
		if e.Seq > req.AfterSeq && e.Seq <= req.AfterSeq+historyPageSize {
			out.Events = append(out.Events, e)
		}
	}
	return out, nil
}

func TestSessionCompletionBetweenSnapshots(t *testing.T) {
	for _, kind := range []string{"turn_end", "usage", "permission", "updater"} {
		m := conversationModel()
		client := &completionClient{events: []*api.Event{{Seq: 150, RunId: "r", Kind: kind}}}
		m.client = client
		s := &api.Session{Id: "remote::other", RunId: "r", State: "idle", LastSeq: 10}
		m.observeSessions([]*api.Session{s})
		s.LastSeq = 300
		cmd := m.observeSessions([]*api.Session{s})
		if cmd == nil || client.calls != 0 {
			t.Fatal("history scan blocked UI or was not scheduled")
		}
		if m.observeSessions([]*api.Session{s}) != nil {
			t.Fatal("duplicate in-flight scan")
		}
		// A single command is returned directly by tea.Batch.
		m.Update(cmd())
		want := " "
		if kind == "turn_end" {
			want = "+"
		}
		if got := ansi.Strip(m.sessionIndicator(s)); got != want {
			t.Fatal("wrong completion classification", kind, got)
		}
		if client.calls < 2 {
			t.Fatal("test failed to exercise paging")
		}
		if m.observeSessions([]*api.Session{s}) != nil {
			t.Fatal("unchanged journal fetched again")
		}
	}
}

func TestSessionCompletionScopeAndStaleProbe(t *testing.T) {
	m := conversationModel()
	m.client = &completionClient{events: []*api.Event{{Seq: 20, RunId: "r", Kind: "turn_end"}}}
	a := &api.Session{Id: "work::same", State: "idle", RunId: "r", LastSeq: 10}
	b := &api.Session{Id: "home::same", State: "idle", RunId: "r", LastSeq: 10}
	m.observeSessions([]*api.Session{a, b})
	a.LastSeq = 20
	cmd := m.observeSessions([]*api.Session{a, b})
	m.sessionActivity[a.Id].seen = 20 // User read the result before the probe arrived.
	m.Update(cmd())
	if m.sessionIndicator(a) != " " || m.sessionIndicator(b) != " " {
		t.Fatal("late probe resurrected read marker or crossed connections")
	}
	a.LastSeq = 30
	cmd = m.observeSessions([]*api.Session{a, b})
	a.RunId, a.LastSeq = "replacement", 31
	m.observeSessions([]*api.Session{a, b})
	m.Update(cmd())
	if m.sessionActivity[a.Id].run != "replacement" || m.sessionIndicator(a) != " " {
		t.Fatal("stale probe affected replacement run")
	}
}

func TestPanelStatusVisibleWithLongSessionName(t *testing.T) {
	m := panelModel()
	m.current().Alias = strings.Repeat("long", 30)
	m.current().State = "working"
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	text := ansi.Strip(m.panelScreen())
	if !strings.Contains(text, "⣟") || !strings.Contains(text, "claude") || strings.Contains(text, "working") {
		t.Fatal("name clipped the status/provider", text)
	}
	m.current().State = "idle"
	if strings.Contains(m.panelScreen(), "idle") || strings.Contains(m.panelScreen(), "⣟") {
		t.Fatal("idle row has activity marker")
	}
}
