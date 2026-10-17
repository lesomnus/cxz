package tui

import (
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/transcripthistory"
)

// seed stands in for a session's runtime: the records it wrote, and the rows its
// projection makes of them. A test that wrote rows by hand would assert against
// a shape nothing produces -- which is how the two readings of a conversation
// drifted apart in the first place (cxz#155). The browser fixture serves rows
// the same way, from the same package.
func seed(m *model, id string, events ...*api.Event) {
	if m.events == nil {
		m.events = map[string][]*api.Event{}
	}
	m.events[id] = events
	setRows(m, id, events)
}

func setRows(m *model, id string, events []*api.Event) {
	if m.rows == nil {
		m.rows = map[string][]*api.Event{}
	}
	m.rows[id] = rowsOf(events)
}

// rowsOf is the projection's answer: the latest version of each row, in reading
// order. The stream publishes a row again when a later record corrects it, and
// a reader replaces by sequence; this is that reader, settled.
func rowsOf(events []*api.Event) []*api.Event {
	var rows []*api.Event
	at := map[uint64]int{}
	for _, v := range transcripthistory.VersionsFromEvents(events, 0) {
		if i, ok := at[v.Event.Seq]; ok {
			rows[i] = v.Event
			continue
		}
		at[v.Event.Seq] = len(rows)
		rows = append(rows, v.Event)
	}
	return rows
}

// live delivers a batch of journal writes the way a runtime does: the records
// on the event stream, and the rows its projection makes of them on the row
// stream. Both are the same writes; a test that sent only one of them would be
// asserting against half of what a client is told.
//
// The rows are built from the whole slice, because the projection is stateful:
// a result only becomes part of its tool's row beside the call it answers.
func live(m *model, id string, events ...*api.Event) {
	for _, e := range events {
		m.Update(received{id: id, event: e})
	}
	updated := uint64(0)
	for _, e := range events {
		if e.Seq > updated {
			updated = e.Seq
		}
	}
	m.Update(liveRows{id: id, rows: rowsOf(events), updated: updated, epoch: m.watchEpoch})
}
