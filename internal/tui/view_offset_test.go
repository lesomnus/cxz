package tui

import (
	"fmt"
	"testing"

	"github.com/lesomnus/cxz/api"
)

// The viewport pulls its offset back when it passes the last line, but not when
// it passes the last line the view can scroll to. Between the two the
// conversation stops partway down the screen with the rest blank, and AtBottom
// reports true -- so the next rebuild believes the reader is following the end
// and jumps there, which is what the reader sees when they scroll.
func TestConversationNeverStopsPartwayDownTheView(t *testing.T) {
	loaded := func() (*model, []*api.Event) {
		m := conversationModel()
		var events []*api.Event
		for i := uint64(1); i <= 80; i++ {
			events = append(events, &api.Event{Seq: i, RunId: "run", Kind: "assistant", Text: fmt.Sprintf("line %d", i)})
		}
		m.applyHistoryPage(historyPage{id: "s", start: 0, initial: true, events: events})
		return m, events
	}
	past := func(t *testing.T, m *model) {
		t.Helper()
		if over := m.view.YOffset - max(0, m.view.TotalLineCount()-m.view.Height); over > 0 {
			t.Fatalf("offset sits %d rows past the end of %d rows in a view of %d",
				over, m.view.TotalLineCount(), m.view.Height)
		}
	}
	show := func(m *model, events []*api.Event) {
		m.events["s"] = events
		m.dropUnloadedRenderCaches()
		m.render()
	}

	t.Run("the view grows", func(t *testing.T) {
		m, _ := loaded()
		// One row back from the end: the reader is not following it.
		m.view.SetYOffset(m.view.TotalLineCount() - m.view.Height - 1)
		if m.view.AtBottom() {
			t.Fatal("the reader is still following the end")
		}
		// A panel closes and the conversation gets taller, with nothing
		// rebuilding the transcript afterwards.
		m.height = 40
		m.resize()
		past(t, m)
	})

	t.Run("the conversation shrinks", func(t *testing.T) {
		m, events := loaded()
		show(m, events[:74])
		short := m.view.TotalLineCount()
		show(m, events)
		// The last row of what is about to be left: valid now, and past the end
		// once the rows below it fold away.
		m.view.SetYOffset(short - 1)
		if m.view.AtBottom() {
			t.Fatal("the reader is still following the end")
		}
		show(m, events[:74])
		past(t, m)
	})
}
