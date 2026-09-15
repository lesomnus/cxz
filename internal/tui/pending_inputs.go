package tui

import (
	"github.com/lesomnus/cxz/api"
	"time"
)

func (m *model) queueInput(id, run, request, text string) {
	if m.pendingInputs == nil {
		m.pendingInputs = map[string][]*api.Event{}
	}
	m.pendingInputs[id] = append(m.pendingInputs[id], &api.Event{SessionId: id, RunId: run, RequestId: request, Kind: "input", Text: text, TimeMs: time.Now().UnixMilli()})
	m.render()
	m.view.GotoBottom()
}
func (m *model) removePendingInput(id, request string) {
	if m.pendingInputs == nil {
		return
	}
	kept := m.pendingInputs[id][:0]
	for _, e := range m.pendingInputs[id] {
		if e.RequestId != request {
			kept = append(kept, e)
		}
	}
	m.pendingInputs[id] = kept
}

// Local echoes never enter the journal or mutate its sequence cursor. Reconcile
// by request ID, not text, so repeated identical messages remain distinct.
func (m *model) transcriptEvents(id string) []*api.Event {
	if len(m.pendingInputs[id]) == 0 {
		return m.events[id]
	}
	confirmed := map[string]bool{}
	for _, e := range m.events[id] {
		if e.Kind == "input" {
			confirmed[e.RunId+"/"+e.RequestId] = true
		}
	}
	pending := m.pendingInputs[id][:0]
	for _, e := range m.pendingInputs[id] {
		if !confirmed[e.RunId+"/"+e.RequestId] {
			pending = append(pending, e)
		}
	}
	m.pendingInputs[id] = pending
	if len(pending) == 0 {
		return m.events[id]
	}
	out := append([]*api.Event{}, m.events[id]...)
	for _, e := range pending {
		at := len(out)
		for at > 0 && out[at-1].TimeMs > e.TimeMs {
			at--
		}
		out = append(out, nil)
		copy(out[at+1:], out[at:])
		out[at] = e
	}
	return out
}

func (m *model) isPendingInput(e *api.Event) bool {
	for _, pending := range m.pendingInputs[e.SessionId] {
		if pending == e {
			return true
		}
	}
	return false
}
