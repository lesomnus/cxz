package tui

import "github.com/lesomnus/cxz/api"

// Journal entries are immutable. Reuse completed rows across live tool output,
// resource refreshes and other events that don't alter earlier conversation.
type inputRenderKey struct {
	event *api.Event
	width int
}
type summaryRenderKey struct {
	event, usage *api.Event
	started      int64
	width        int
}

func (m *model) cachedInput(s *api.Session, e *api.Event, width int) string {
	key := inputRenderKey{e, width}
	if body, ok := m.renderedInputs[key]; ok {
		return body
	}
	if m.renderedInputs == nil {
		m.renderedInputs = map[inputRenderKey]string{}
	}
	body := eventView(s, e, width)
	m.renderedInputs[key] = body
	return body
}
func (m *model) cachedSummary(e, usage *api.Event, started int64, width int) string {
	key := summaryRenderKey{e, usage, started, width}
	if body, ok := m.renderedSummaries[key]; ok {
		return body
	}
	if m.renderedSummaries == nil {
		m.renderedSummaries = map[summaryRenderKey]string{}
	}
	body := turnSummary(e, usage, started, width)
	m.renderedSummaries[key] = body
	return body
}
