package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"google.golang.org/grpc"
)

// A bounded journal, with the newer turns heavy enough that one page of them
// fills the scroll window on its own -- a session that started running long
// commands, which is when the window has to choose what to give up.
type journalClient struct {
	api.SessionsClient
	total, heavy uint64
}

func (c *journalClient) History(_ context.Context, r *api.WatchRequest, _ ...grpc.CallOption) (*api.EventBatch, error) {
	b := &api.EventBatch{}
	if r.AfterSeq >= c.total {
		return b, nil
	}
	b.Events = trimFixture(r.AfterSeq, int(min(128, c.total-r.AfterSeq)))
	for _, e := range b.Events {
		if e.Seq > c.heavy && e.Kind == "assistant" {
			e.Text = strings.Repeat(e.Text, 12)
		}
	}
	return b, nil
}

// Reading back through a long journal detaches the window from the live tail.
// Scrolling down again then fetches newer pages, and the window has to evict to
// hold them -- but the reader is standing at the old end, which is exactly what
// evicting the old end takes away. Losing it drops them at the top of whatever
// survived, several screens from where they were and past events they never
// saw, so scrolling down reads as jumping backwards into old conversation.
func TestScrollingDownKeepsTheReaderInPlace(t *testing.T) {
	m := conversationModel()
	m.windowPolicy = &historypolicy.Window{MiB: 1}
	m.client = &journalClient{total: 20000, heavy: 16600}
	m.applyHistoryPage(historyPage{id: "s", start: 19872, initial: true, events: trimFixture(19872, 128)})
	for range 14 {
		m.historyWindow("s").direction = -1
		m.view.GotoTop()
		cmd := m.loadOlderHistory()
		if cmd == nil {
			break
		}
		m.Update(cmd())
	}
	if !m.historyWindow("s").detached {
		t.Fatal("reading back never detached the window from the tail")
	}
	m.view.GotoBottom()
	before := ansi.Strip(m.view.View())
	anchor, loaded := m.historyAnchor(), m.events["s"]
	tail := loaded[len(loaded)-1].Seq
	_, cmd := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress, X: m.contentOffset() + 5, Y: 3})
	if cmd == nil {
		t.Fatal("scrolling down at the loaded tail did not fetch newer history")
	}
	m.Update(cmd())
	loaded = m.events["s"]
	if loaded[len(loaded)-1].Seq <= tail {
		t.Fatal("no newer events were taken")
	}
	if ansi.Strip(m.view.View()) != before {
		t.Fatal("loading newer history moved the reader off what they were reading")
	}
	// Contiguous with what they had: the window may not open past the end of
	// the old one, or the events between are skipped without being shown.
	if loaded[0].Seq > tail {
		t.Fatalf("window skipped events %d..%d", tail, loaded[0].Seq)
	}
	if float64(loaded[0].Seq) > anchor {
		t.Fatalf("evicted the turn the reader was in: window starts at %d, reader at %.1f", loaded[0].Seq, anchor)
	}
}
