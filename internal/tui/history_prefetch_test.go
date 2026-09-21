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
			m.historyOpening = map[string]bool{"s": true}
			m.watchID, m.watchEpoch = "s", 2
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
			if !m.historyOpening["s"] {
				t.Fatal("stale watcher cleared the skeleton")
			}
			m.applyHistoryPage(historyPage{id: "s", initial: true, epoch: 2, events: []*api.Event{{Seq: 1, Kind: "assistant", Text: "Loaded answer"}}})
			if m.historyOpening["s"] || !strings.Contains(m.conversationView(), "Loaded answer") {
				t.Fatal("skeleton did not yield to loaded content")
			}
			m.historyOpening["s"] = true
			m.Update(disconnected{id: "s", err: errors.New("offline")})
			if m.historyOpening["s"] {
				t.Fatal("failed initial load left the skeleton active")
			}
		}
	}
}

func TestHistorySkeletonWaitsThroughBookkeepingPages(t *testing.T) {
	m := conversationModel()
	m.historyOpening = map[string]bool{"s": true}
	m.watchID, m.watchEpoch = "s", 2
	// An event queued by an earlier watcher must not expose the empty transcript.
	m.Update(received{id: "s", event: &api.Event{Seq: 513, Kind: "usage"}})
	if !strings.Contains(m.conversationView(), "Loading conversation") {
		t.Fatal("a queued bookkeeping event replaced the skeleton")
	}
	var tail []*api.Event
	for seq := uint64(385); seq <= 512; seq++ {
		tail = append(tail, &api.Event{Seq: seq, Kind: "state", Text: "idle"})
	}
	m.Update(historyPage{id: "s", initial: true, epoch: 2, start: 384, events: tail})
	if !m.historyOpening["s"] || m.historyLoading["s"] != 384 || !strings.Contains(m.conversationView(), "Loading conversation") {
		t.Fatal("an invisible tail page ended loading before any conversation arrived")
	}
	var updates []*api.Event
	for seq := uint64(257); seq <= 384; seq++ {
		updates = append(updates, &api.Event{Seq: seq, Kind: "update", Text: "agent update completed"})
	}
	m.Update(historyPage{id: "s", end: 384, start: 256, events: updates})
	if m.view.YOffset <= 6*m.view.Height {
		t.Fatal("fixture must exceed the normal prefetch runway")
	}
	if !m.historyOpening["s"] || m.historyLoading["s"] != 256 || !strings.Contains(m.conversationView(), "Loading conversation") {
		t.Fatal("visible bookkeeping stopped loading or replaced the skeleton")
	}
	m.Update(historyPage{id: "s", end: 256, start: 128, events: []*api.Event{{Seq: 256, Kind: "assistant", Text: "Actual conversation"}}})
	if m.historyOpening["s"] || !m.view.AtBottom() || m.historyLoading["s"] != 0 || m.cursor["s"] != 513 {
		t.Fatal("conversation arrival did not end opening at the latest position or stop eager paging")
	}
	m.view.GotoTop()
	if !strings.Contains(m.conversationView(), "Actual conversation") {
		t.Fatal("loaded conversation was not revealed")
	}
}

func TestHistoryOpeningSurvivesSessionSwitch(t *testing.T) {
	m := conversationModel()
	m.sessions = append(m.sessions, &api.Session{Id: "other", Agent: "claude"})
	m.historyOpening = map[string]bool{"s": true}
	m.watchID, m.watchEpoch = "s", 2
	m.Update(historyPage{id: "s", initial: true, epoch: 2, start: 256, events: []*api.Event{{Seq: 384, Kind: "state"}}})
	m.selected, m.watchID = 1, "other"
	m.Update(historyPage{id: "s", start: 128, end: 256, events: []*api.Event{{Seq: 256, Kind: "state"}}})
	m.render()
	if strings.Contains(m.conversationView(), "Loading conversation") || m.historyLoading["s"] != 0 {
		t.Fatal("inactive session affected the selected conversation or kept fetching")
	}
	m.selected, m.watchID = 0, "s"
	m.render()
	m.Update(tick(time.Now()))
	if !strings.Contains(m.conversationView(), "Loading conversation") || m.historyLoading["s"] != 128 {
		t.Fatal("returning to an unfinished history load did not resume the skeleton and paging")
	}
}

func TestHistoryOpeningEndsOnExhaustionOrError(t *testing.T) {
	for _, fail := range []bool{false, true} {
		m := conversationModel()
		m.historyOpening = map[string]bool{"s": true}
		m.watchID, m.watchEpoch = "s", 2
		m.Update(historyPage{id: "s", initial: true, epoch: 2, start: 128, events: []*api.Event{{Seq: 256, Kind: "state"}}})
		page := historyPage{id: "s", start: 0, end: 128, events: []*api.Event{{Seq: 128, Kind: "state"}}}
		if fail {
			page.err = errors.New("offline")
		}
		m.Update(page)
		if m.historyOpening["s"] || strings.Contains(m.conversationView(), "Loading conversation") || m.historyLoading["s"] != 0 {
			t.Fatal("exhaustion or failure left the skeleton active")
		}
		if fail && !strings.Contains(m.notice, "offline") {
			t.Fatal("history error was not surfaced")
		}
		if !fail && !strings.Contains(m.conversationView(), "Start a conversation") {
			t.Fatal("an empty journal did not yield to the empty conversation view")
		}
	}
}

func TestHistorySkeletonUsesThreeCompactParagraphs(t *testing.T) {
	profile := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(profile)
	for _, colors := range []termenv.Profile{termenv.Ascii, termenv.ANSI256} {
		lipgloss.SetColorProfile(colors)
		for _, width := range []int{40, 80, 200} {
			rows := strings.Split(historySkeleton(width, 60, 1), "\n")
			paragraphs, bars := 0, 0
			previous := false
			for y, row := range rows {
				bar := strings.Contains(row, "░") || strings.Contains(row, "\x1b[48;5;235m")
				if bar {
					bars++
					if !previous {
						paragraphs++
					}
					if y >= 13 || ansi.StringWidth(row) > min(width, 66) {
						t.Fatal("skeleton filled the viewport instead of a compact area at the top")
					}
				}
				previous = bar
			}
			if paragraphs != 3 || bars != 9 || len(rows) != 60 {
				t.Fatalf("skeleton layout: %d paragraphs, %d bars, %d viewport rows", paragraphs, bars, len(rows))
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
