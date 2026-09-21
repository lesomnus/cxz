package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
	"google.golang.org/grpc"
)

func historyFixture(start uint64, count int) []*api.Event {
	var events []*api.Event
	for i := 0; i < count; i++ {
		seq := start + uint64(i) + 1
		events = append(events, &api.Event{Seq: seq, Kind: "assistant", Text: fmt.Sprintf("response %d\nsecond line", seq)})
	}
	return events
}

func TestHistoryPrefetchStartsBeforeLoadedEdge(t *testing.T) {
	m := conversationModel()
	m.client = &pagedClient{}
	m.Update(historyPage{id: "s", start: 512, initial: true, events: historyFixture(512, 128)})
	if m.view.YOffset <= 6*m.view.Height || m.historyLoading["s"] != 512 {
		t.Fatal("initial tail did not schedule one older page while still at the bottom")
	}
	m.applyHistoryPage(historyPage{id: "s", start: 384, end: 512, events: historyFixture(384, 128)})
	if m.loadOlderHistory() != nil || !m.view.AtBottom() {
		t.Fatal("prefetch eagerly drained the whole journal or moved the viewport")
	}
	m.view.SetYOffset(4 * m.view.Height)
	visible := m.view.View()
	cmd := m.loadOlderHistory()
	if cmd == nil || m.loadOlderHistory() != nil {
		t.Fatal("prefetch did not start several screens before the edge, or duplicated the request")
	}
	m.applyHistoryPage(cmd().(historyPage))
	if got := m.view.View(); got != visible {
		t.Fatal("prefetch changed the visible text")
	}
	if m.historyStart["s"] != 256 || m.cursor["s"] != 640 {
		t.Fatal("prefetch changed the replay cursor")
	}
}

type slowHistoryClient struct {
	api.SessionsClient
	requested chan struct{}
	release   chan struct{}
	events    []*api.Event
}

func (c *slowHistoryClient) History(ctx context.Context, _ *api.WatchRequest, _ ...grpc.CallOption) (*api.EventBatch, error) {
	close(c.requested)
	select {
	case <-c.release:
		return &api.EventBatch{Events: c.events}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestHistoryPreparationKeepsUIResponsive(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	m := conversationModel()
	m.applyHistoryPage(historyPage{id: "s", start: 256, initial: true, events: historyFixture(256, 64)})
	m.view.SetYOffset(4 * m.view.Height)
	client := &slowHistoryClient{requested: make(chan struct{}), release: make(chan struct{}), events: []*api.Event{
		{Seq: 129, Kind: "assistant", Text: "## Earlier answer\n\n```go\npackage main\nfunc main() {}\n```"},
		{Seq: 130, RunId: "run", RequestId: "bash", Kind: "tool_call", Text: "Bash", Payload: []byte(`{"command":"for i in 1 2 3; do echo $i; done"}`)},
		{Seq: 131, RunId: "run", RequestId: "bash", Kind: "tool_result", Text: "done"},
	}}
	m.client = client
	cmd := m.loadOlderHistory()
	if cmd == nil {
		t.Fatal("missing prefetch")
	}
	done := make(chan historyPage, 1)
	go func() { done <- cmd().(historyPage) }()
	<-client.requested
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("draft during loading")})
	if m.input.Value() != "draft during loading" || m.historyStart["s"] != 256 {
		t.Fatal("network wait blocked input or prematurely applied history")
	}
	close(client.release)
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	pulse := time.NewTicker(time.Millisecond)
	defer pulse.Stop()
	for {
		select {
		case page := <-done:
			if page.err != nil || len(page.prepared) != 1 || len(page.toolBodies) != 1 {
				t.Fatal("history arrived without prepared Markdown and tool rows", page.err)
			}
			if _, exists := m.renderedResponses[page.events[0]]; exists || len(m.renderedTools) != 0 {
				t.Fatal("worker mutated the UI caches")
			}
			visible := m.view.View()
			m.applyHistoryPage(page)
			if m.view.View() != visible || m.input.Value() != "draft during loading" || len(m.renderedTools) != 1 {
				t.Fatal("prepared page was not reused or changed the viewport/draft")
			}
			return
		case <-pulse.C:
			m.Update(pulseTick{})
			m.render()
			m.View()
		case <-deadline.C:
			t.Fatal("history preparation did not finish")
		}
	}
}

func TestHistoryPrefetchRejectsStalePagesAndStopsOnError(t *testing.T) {
	m := conversationModel()
	m.historyStart = map[string]uint64{"s": 512}
	m.historyLoading = map[string]uint64{"s": 512}
	if m.applyHistoryPage(historyPage{id: "s", start: 640, end: 768}) || m.historyLoading["s"] != 512 {
		t.Fatal("stale page replaced the active request")
	}
	m.Update(historyPage{id: "s", start: 384, end: 512, err: errors.New("offline")})
	if m.historyLoading["s"] != 0 || m.historyStart["s"] != 512 || !strings.Contains(m.notice, "offline") {
		t.Fatal("failed prefetch advanced history or automatically retried")
	}
}

func TestInitialHistorySkeletonLifecycle(t *testing.T) {
	profile := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(profile)
	for _, colors := range []termenv.Profile{termenv.Ascii, termenv.ANSI256} {
		lipgloss.SetColorProfile(colors)
		for _, width := range []int{40, 80, 200} {
			m := conversationModel()
			m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
			m.historyOpening, m.watchID, m.watchEpoch = true, "s", 2
			before := m.conversationView()
			if !strings.Contains(ansi.Strip(before), "Loading conversation") || strings.Contains(before, "Start a conversation") {
				t.Fatal("missing initial skeleton")
			}
			m.Update(pulseTick{})
			if before == m.conversationView() {
				t.Fatal("skeleton did not animate")
			}
			rows := strings.Split(m.View(), "\n")
			if len(rows) != 24 {
				t.Fatal("skeleton changed screen height")
			}
			for _, row := range rows {
				if ansi.StringWidth(row) != width {
					t.Fatal("skeleton overflowed the conversation")
				}
			}
			m.applyHistoryPage(historyPage{id: "s", initial: true, epoch: 1})
			if !m.historyOpening {
				t.Fatal("stale watcher cleared the skeleton")
			}
			m.applyHistoryPage(historyPage{id: "s", initial: true, epoch: 2, events: []*api.Event{{Seq: 1, Kind: "assistant", Text: "Loaded answer"}}})
			if m.historyOpening || !strings.Contains(m.conversationView(), "Loaded answer") {
				t.Fatal("skeleton did not yield to loaded content")
			}
			m.historyOpening = true
			m.Update(disconnected{id: "s", err: errors.New("offline")})
			if m.historyOpening {
				t.Fatal("failed initial load left the skeleton active")
			}
		}
	}
}

func BenchmarkHistoryPageApply(b *testing.B) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	var events []*api.Event
	for i := uint64(1); i <= 128; i++ {
		e := &api.Event{Seq: i, RunId: "run", Kind: "assistant", Text: "## Explanation\n\n```go\npackage main\nfunc main() { println(\"hello\") }\n```"}
		if i%2 == 0 {
			e.Kind, e.Text, e.RequestId = "tool_call", "Bash", fmt.Sprint(i)
			e.Payload = []byte(`{"command":"for item in *.go; do echo \"$item\"; done"}`)
		}
		events = append(events, e)
	}
	for _, prepared := range []bool{false, true} {
		b.Run(fmt.Sprintf("prepared=%v", prepared), func(b *testing.B) {
			page := historyPage{id: "s", start: 0, end: 128, events: events}
			if prepared {
				m := conversationModel()
				page = m.prepareHistoryPage(context.Background(), page, "claude", m.view.Width)
			}
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				m := conversationModel()
				m.applyHistoryPage(historyPage{id: "s", start: 128, initial: true, events: historyFixture(128, 128)})
				m.view.SetYOffset(4 * m.view.Height)
				b.StartTimer()
				m.applyHistoryPage(page)
			}
		})
	}
}
