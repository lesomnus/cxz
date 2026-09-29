package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"google.golang.org/grpc"
)

type windowClient struct {
	api.SessionsClient
	events []*api.Event
	floor  uint64
}

func (c *windowClient) History(_ context.Context, r *api.WatchRequest, _ ...grpc.CallOption) (*api.EventBatch, error) {
	b := &api.EventBatch{}
	if r.AfterSeq < c.floor {
		b.Events = append(b.Events, &api.Event{Seq: c.floor, Kind: core.HistoryTrimmedKind, Payload: []byte(fmt.Sprintf(`{"through":%d}`, c.floor))})
	}
	for _, e := range c.events {
		if e.Seq > r.AfterSeq && e.Seq > c.floor && len(b.Events) < 128 {
			b.Events = append(b.Events, e)
		}
	}
	return b, nil
}
func windowTurns(n int) []*api.Event {
	var es []*api.Event
	for i := range n {
		for _, kind := range []string{"input", "assistant", "turn_end"} {
			es = append(es, &api.Event{SessionId: "s", RunId: "run", Seq: uint64(len(es) + 1), Kind: kind, Text: fmt.Sprintf("%s %d", kind, i)})
		}
	}
	return es
}
func TestHistoryWindowOlderNewerLiveAndLatest(t *testing.T) {
	m := conversationModel()
	m.windowPolicy = &historypolicy.Window{Turns: 5}
	c := &windowClient{events: windowTurns(70)}
	m.client = c
	m.applyHistoryPage(historyPage{id: "s", initial: true, start: 150, events: c.events[150:]})
	if len(m.events["s"]) != 15 || m.historyStart["s"] != 195 || m.cursor["s"] != 210 {
		t.Fatal("tail window", len(m.events["s"]), m.historyStart, m.cursor)
	}
	m.view.GotoTop()
	cmd := m.loadOlderHistory()
	if cmd == nil {
		t.Fatal("older history unavailable")
	}
	m.Update(cmd())
	w := m.historyWindow("s")
	if !w.detached || len(m.events["s"]) > 15 {
		t.Fatal("older window unbounded", len(m.events["s"]), w)
	}
	olderFirst := m.events["s"][0].Seq
	incoming := &api.Event{SessionId: "s", Seq: 211, Kind: "assistant", Text: "new while browsing"}
	c.events = append(c.events, incoming)
	m.receiveEvent(received{"s", incoming}, false)
	if m.cursor["s"] != 211 || m.events["s"][0].Seq != olderFirst || m.events["s"][len(m.events["s"])-1].Seq >= 211 {
		t.Fatal("live event displaced older window")
	}
	// A newer page is available when scrolling down; latest jumps without replay.
	w.direction = 1
	m.view.GotoBottom()
	cmd = m.requestNewerHistory(false)
	if cmd == nil {
		t.Fatal("newer page unavailable")
	}
	m.Update(cmd())
	if w.detached {
		cmd = m.requestNewerHistory(true)
		if cmd == nil {
			t.Fatal("latest unavailable")
		}
		m.Update(cmd())
	}
	if w.detached || m.events["s"][len(m.events["s"])-1].Seq != 211 || len(m.events["s"]) > 16 {
		t.Fatal("latest tail lost", w, len(m.events["s"]))
	}
}
func TestHistoryFloorStopsPagingAndRejectsLateRows(t *testing.T) {
	m := conversationModel()
	c := &windowClient{events: windowTurns(20), floor: 45}
	m.client = c
	m.applyHistoryPage(historyPage{id: "s", initial: true, start: 45, events: c.events[45:]})
	m.view.GotoTop()
	cmd := m.loadOlderHistory()
	if cmd == nil {
		t.Fatal("expected boundary lookup")
	}
	m.Update(cmd())
	if m.historyWindow("s").floor != 45 || m.historyStart["s"] != 45 || m.loadOlderHistory() != nil {
		t.Fatal("boundary paging loop")
	}
	m.receiveEvent(received{"s", &api.Event{Seq: 61, Kind: core.HistoryTrimmedKind, Payload: []byte(`{"through":51}`)}}, true)
	for _, e := range m.events["s"] {
		if e.Seq <= 51 {
			t.Fatal("trimmed rows still loaded")
		}
	}
	if !strings.Contains(m.view.View(), "Earlier display history") {
		m.view.GotoTop()
		if !strings.Contains(m.view.View(), "Earlier display history") {
			t.Fatal("missing retention notice")
		}
	}
}
func TestHistoryWindowBytesEvictsRenderCaches(t *testing.T) {
	m := conversationModel()
	m.windowPolicy = &historypolicy.Window{MiB: 1, Turns: 200}
	events := windowTurns(6)
	for _, e := range events {
		if e.Kind == "assistant" {
			e.Text = strings.Repeat("x", 300000)
		}
	}
	m.events["s"] = events
	kept := events[len(events)-1]
	m.renderedResponses = map[*api.Event]renderedResponse{events[1]: {body: "old cached output"}, kept: {body: "still loaded"}}
	if !m.limitHistory("s", false, 0) || len(m.events["s"]) != 9 {
		t.Fatal("byte limit failed", len(m.events["s"]))
	}
	// Only evicted events lose their rows. Re-rendering the survivors would
	// redo the whole window on the UI goroutine at every trim.
	if _, ok := m.renderedResponses[events[1]]; ok {
		t.Fatal("cached rows outlived the event they were keyed on")
	}
	if m.renderedResponses[kept].body != "still loaded" {
		t.Fatal("trimming discarded rows of an event that is still loaded")
	}
	// One active/oversized turn is kept whole instead of breaking tool pairs.
	m.events["s"] = events[:3]
	m.events["s"][1].Text = strings.Repeat("x", 2*historypolicy.MiB)
	m.limitHistory("s", false, 0)
	if len(m.events["s"]) != 3 {
		t.Fatal("oversized turn was split")
	}
}

func TestLiveHistoryStaysBoundedAcrossThousandsOfTurns(t *testing.T) {
	m := conversationModel()
	m.windowPolicy = &historypolicy.Window{Turns: 20}
	for _, e := range windowTurns(2000) {
		m.receiveEvent(received{"s", e}, false)
	}
	if len(m.events["s"]) != 60 || m.cursor["s"] != 6000 || m.events["s"][0].Seq != 5941 {
		t.Fatal("live history grew beyond window", len(m.events["s"]), m.cursor["s"])
	}
}
func TestHistoryWindowDoesNotSplitSteeredTurn(t *testing.T) {
	m := conversationModel()
	m.windowPolicy = &historypolicy.Window{MiB: 1, Turns: 1}
	events := windowTurns(1)
	events = append(events, &api.Event{Seq: 4, Kind: "input", Text: "start active turn"}, &api.Event{Seq: 5, Kind: "tool_call", RequestId: "active"}, &api.Event{Seq: 6, Kind: "input", Text: "steer active turn"}, &api.Event{Seq: 7, Kind: "assistant", Text: strings.Repeat("x", historypolicy.MiB)})
	m.events["s"] = events
	m.limitHistory("s", false, 0)
	if len(m.events["s"]) != 4 || m.events["s"][0].Seq != 4 || m.events["s"][1].RequestId != "active" {
		t.Fatal("active turn/tool pair split", len(m.events["s"]))
	}
}
