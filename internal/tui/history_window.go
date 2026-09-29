package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"google.golang.org/protobuf/proto"
)

type historyWindow struct {
	floor         uint64
	tail          uint64
	detached      bool
	loading       bool
	generation    uint64
	direction     int
	measured      int
	measuredBytes int
	measuredTurns int
	boundaryReady bool
	first         *api.Event
	last          *api.Event
}

func (m *model) historyWindow(id string) *historyWindow {
	if m.historyWindows == nil {
		m.historyWindows = map[string]*historyWindow{}
	}
	if m.historyWindows[id] == nil {
		m.historyWindows[id] = &historyWindow{}
	}
	return m.historyWindows[id]
}
func historyBatchFloor(events []*api.Event) uint64 {
	var n uint64
	for _, e := range events {
		n = max(n, core.HistoryFloor(e.Kind, e.Payload))
	}
	return n
}
func (m *model) clearHistoryRenderCaches() {
	m.renderedResponses = nil
	m.renderedTools = nil
	m.toolActivities = nil
	m.renderedInputs = nil
	m.renderedSummaries = nil
	m.contextStatusCache = nil
	m.quotaParses = nil
	if len(m.hiddenEvents) > 0 {
		keep := map[*api.Event]bool{}
		for _, events := range m.events {
			for _, e := range events {
				if m.hiddenEvents[e] {
					keep[e] = true
				}
			}
		}
		m.hiddenEvents = keep
	}
}
func (m *model) applyHistoryFloor(id string, floor uint64) {
	w := m.historyWindow(id)
	if floor <= w.floor {
		return
	}
	w.floor = floor
	w.measured = 0
	w.first = nil
	w.last = nil
	var keep []*api.Event
	for _, e := range m.events[id] {
		if e.Seq > floor {
			keep = append(keep, e)
		}
	}
	m.events[id] = keep
	if m.historyStart == nil {
		m.historyStart = map[string]uint64{}
	}
	m.historyStart[id] = max(m.historyStart[id], floor)
	m.clearHistoryRenderCaches()
}

// Bound by whole input turns. One oversized turn remains intact. Browsing older
// pages evicts the newer end; live following evicts the older end.
func (m *model) limitHistory(id string, older bool) bool {
	events := m.events[id]
	p := m.windowPreference()
	maxBytes, maxTurns := p.Limits()
	w := m.historyWindow(id)
	if len(events) == 0 {
		w.measured = 0
		w.first = nil
		w.last = nil
		return false
	}
	if w.measured+1 == len(events) && w.measured > 0 && w.first == events[0] && w.last == events[len(events)-2] {
		e := events[len(events)-1]
		w.measuredBytes += proto.Size(e)
		if e.Kind == "input" {
			if w.boundaryReady {
				w.measuredTurns++
			}
			w.boundaryReady = false
		}
		if historyTurnClosed(e) {
			w.boundaryReady = true
		}
	} else {
		w.measuredBytes, w.measuredTurns = 0, 1
		w.boundaryReady = false
		for i, e := range events {
			w.measuredBytes += proto.Size(e)
			if e.Kind == "input" {
				if i > 0 && w.boundaryReady {
					w.measuredTurns++
				}
				w.boundaryReady = false
			}
			if historyTurnClosed(e) {
				w.boundaryReady = true
			}
		}
	}
	w.measured = len(events)
	w.first = events[0]
	w.last = events[len(events)-1]
	if w.measuredTurns <= 1 || w.measuredBytes <= maxBytes && w.measuredTurns <= maxTurns {
		return false
	}
	starts := []int{0}
	bytes := make([]int, len(events)+1)
	ready := false
	for i, e := range events {
		bytes[i+1] = bytes[i] + proto.Size(e)
		if e.Kind == "input" {
			if i > 0 && ready {
				starts = append(starts, i)
			}
			ready = false
		}
		if historyTurnClosed(e) {
			ready = true
		}
	}
	if len(events) == 0 {
		return false
	}
	starts = append(starts, len(events))
	lo, hi := 0, len(starts)-1
	for hi-lo > 1 && (hi-lo > maxTurns || bytes[starts[hi]]-bytes[starts[lo]] > maxBytes) {
		if older {
			hi--
		} else {
			lo++
		}
	}
	from, to := starts[lo], starts[hi]
	if from == 0 && to == len(events) {
		return false
	}
	w.measured = 0
	w.first = nil
	w.last = nil
	if older && to < len(events) {
		w.detached = true
		w.tail = events[to-1].Seq
	}
	m.events[id] = append([]*api.Event(nil), events[from:to]...)
	if m.historyStart == nil {
		m.historyStart = map[string]uint64{}
	}
	if from > 0 {
		m.historyStart[id] = max(w.floor, m.events[id][0].Seq-1)
	}
	m.clearHistoryRenderCaches()
	return true
}
func (m *model) historyAnchor() float64 {
	if len(m.historyPositions) == 0 {
		return 0
	}
	return m.historyPositions[min(m.view.YOffset, len(m.historyPositions)-1)]
}
func (m *model) restoreHistoryAnchor(anchor float64) {
	if anchor == 0 {
		return
	}
	for i, p := range m.historyPositions {
		if p >= anchor {
			m.view.SetYOffset(i)
			return
		}
	}
}

type historyWindowPage struct {
	page       historyPage
	generation uint64
	after      uint64
	latest     bool
}

func (m *model) requestNewerHistory(latest bool) tea.Cmd {
	s := m.current()
	if s == nil {
		return nil
	}
	w := m.historyWindow(s.Id)
	if !w.detached || (!latest && (w.loading || w.direction <= 0)) {
		return nil
	}
	if !latest && m.view.TotalLineCount()-m.view.YOffset > m.view.Height*7 {
		return nil
	}
	events := m.events[s.Id]
	after := w.tail
	if after == 0 && len(events) > 0 {
		after = events[len(events)-1].Seq
	}
	if latest {
		after = max(s.LastSeq, m.cursor[s.Id])
		if after > historyPageSize {
			after -= historyPageSize
		} else {
			after = 0
		}
		w.generation++
		w.direction = 1
		delete(m.historyLoading, s.Id)
	}
	w.loading = true
	id, agent, width, generation := s.Id, s.Agent, m.view.Width, w.generation
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		batch, err := m.fetchHistory(ctx, id, after, "newer")
		page := historyPage{id: id, start: after, err: err}
		if batch != nil {
			page.events = batch.Events
		}
		return historyWindowPage{m.prepareHistoryPage(ctx, page, agent, width), generation, after, latest}
	}
}
func (m *model) applyWindowPage(result historyWindowPage) tea.Cmd {
	p := result.page
	w := m.historyWindow(p.id)
	if w.generation != result.generation {
		return nil
	}
	w.loading = false
	if !result.latest && w.tail != 0 && w.tail != result.after {
		return nil
	}
	if p.err != nil {
		m.showError("History: " + p.err.Error())
		return nil
	}
	anchor := m.historyAnchor()
	m.applyHistoryFloor(p.id, historyBatchFloor(p.events))
	if result.latest {
		m.events[p.id] = nil
		m.historyStart[p.id] = max(result.after, w.floor)
	}
	events := m.events[p.id]
	last := result.after
	if len(events) > 0 {
		last = events[len(events)-1].Seq
	}
	for _, e := range p.events {
		if e.Seq > last && e.Seq > w.floor {
			last = e.Seq
			m.events[p.id] = append(m.events[p.id], e)
		}
	}
	w.tail = last
	w.detached = last < m.cursor[p.id]
	m.mergePreparedHistory(p)
	m.limitHistory(p.id, false)
	if s := m.current(); s != nil && s.Id == p.id {
		m.render()
		if result.latest {
			m.view.GotoBottom()
		} else {
			m.restoreHistoryAnchor(anchor)
		}
	}
	return nil
}

func historyTurnClosed(e *api.Event) bool {
	return e.Kind == "turn_end" || e.Kind == "state" && (e.Text == "idle" || e.Text == "stopped" || e.Text == "failed" || e.Text == "interrupted")
}
