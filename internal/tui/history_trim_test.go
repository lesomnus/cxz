package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"github.com/muesli/termenv"
)

// Turns heavy enough that a few pages fill the byte window, as a real
// conversation of prose, code and tool calls does.
func trimFixture(start uint64, count int) []*api.Event {
	body := strings.Repeat("The supervisor streamed this paragraph back to the client. ", 30)
	var events []*api.Event
	for i := 0; i < count; i++ {
		seq := start + uint64(i) + 1
		e := &api.Event{Seq: seq, RunId: "run"}
		switch seq % 12 {
		case 0:
			e.Kind, e.Text = "input", fmt.Sprintf("please look at item %d", seq)
		case 11:
			e.Kind, e.Text = "turn_end", "end"
		case 7, 8, 9, 10:
			e.Kind, e.Text, e.RequestId = "tool_call", "Bash", fmt.Sprint(seq)
			e.Payload = []byte(`{"command":"for item in *.go; do echo \"$item\"; done"}`)
		default:
			e.Kind = "assistant"
			e.Text = fmt.Sprintf("## Step %d\n\n%s\n\n```go\nfunc main() { println(%d) }\n```\n\n%s", seq, body, seq, body)
		}
		events = append(events, e)
	}
	return events
}

// Page backwards from the tail until the window is full and starts evicting.
func windowAtLimit(t testing.TB, tail uint64) (*model, uint64) {
	m := conversationModel()
	m.windowPolicy = &historypolicy.Window{MiB: 1}
	m.applyHistoryPage(historyPage{id: "s", start: tail, initial: true, events: trimFixture(tail, 128)})
	start := tail
	for range 40 {
		if m.historyWindow("s").detached {
			return m, start
		}
		m.view.SetYOffset(0)
		m.applyHistoryPage(historyPage{id: "s", start: start - 128, end: start, events: trimFixture(start-128, 128)})
		start -= 128
	}
	t.Fatal("window never reached its limit")
	return nil, 0
}

// Scrolling past the window limit evicts the far end of the window on every
// page. Rows of the events that remain, including those the fetch goroutine
// just prepared, must survive that eviction; rebuilding them would put the
// whole window back on the UI goroutine for the rest of the scroll.
func TestTrimKeepsRowsOfEventsStillLoaded(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	m, start := windowAtLimit(t, 8192)
	page := m.prepareHistoryPage(context.Background(), historyPage{id: "s", start: start - 128, end: start, events: trimFixture(start-128, 128)}, "claude", m.view.Width)
	if len(page.prepared) == 0 || len(page.toolBodies) == 0 {
		t.Fatal("nothing was prepared off the UI goroutine")
	}
	// A row that survives the eviction, marked so a rebuild is visible: the
	// cached body is returned verbatim while the cache entry stays valid.
	var marked *api.Event
	for _, e := range m.events["s"] {
		if cached, ok := m.renderedResponses[e]; ok && e.Kind == "assistant" {
			cached.body = "row reused, not rebuilt"
			m.renderedResponses[e] = cached
			marked = e
			break
		}
	}
	if marked == nil {
		t.Fatal("no rendered response to mark")
	}
	before := len(m.events["s"])
	m.view.SetYOffset(0)
	m.applyHistoryPage(page)
	if len(m.events["s"]) >= before+len(page.events) {
		t.Fatal("the page was applied without evicting, so the window was not at its limit")
	}
	loaded := map[*api.Event]bool{}
	for _, e := range m.events["s"] {
		loaded[e] = true
	}
	if !loaded[marked] {
		t.Fatal("the marked event was evicted; mark one from the surviving end")
	}
	if m.renderedResponses[marked].body != "row reused, not rebuilt" {
		t.Fatal("eviction rebuilt the rows of an event that is still loaded")
	}
	for key := range m.renderedResponses {
		if !loaded[key] {
			t.Fatal("rows outlived the event they were keyed on")
		}
	}
	for key := range m.renderedTools {
		if !loaded[key.event] {
			t.Fatal("tool rows outlived the event they were keyed on")
		}
	}
}

func BenchmarkOlderPageAtWindowLimit(b *testing.B) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	for b.Loop() {
		b.StopTimer()
		m, start := windowAtLimit(b, 8192)
		page := m.prepareHistoryPage(context.Background(), historyPage{id: "s", start: start - 128, end: start, events: trimFixture(start-128, 128)}, "claude", m.view.Width)
		m.view.SetYOffset(0)
		b.StartTimer()
		m.applyHistoryPage(page)
	}
}
