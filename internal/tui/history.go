package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

const historyPageSize uint64 = 128

type historyPage struct {
	id      string
	events  []*api.Event
	start   uint64
	end     uint64
	epoch   uint64
	initial bool
	err     error
}

// Journal sequence numbers are contiguous. The existing ascending History API
// returns at most 128 events, so a sequence window also provides reverse paging.
func (m *model) loadOlderHistory() tea.Cmd {
	s := m.current()
	if s == nil || m.view.YOffset > m.view.Height || m.historyStart[s.Id] == 0 || m.historyLoading[s.Id] {
		return nil
	}
	id, end := s.Id, m.historyStart[s.Id]
	start := uint64(0)
	if end > historyPageSize {
		start = end - historyPageSize
	}
	if m.historyLoading == nil {
		m.historyLoading = map[string]bool{}
	}
	m.historyLoading[id] = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		batch, err := m.client.History(ctx, &api.WatchRequest{SessionId: id, AfterSeq: start})
		page := historyPage{id: id, start: start, end: end, err: err}
		if batch != nil {
			for _, e := range batch.Events {
				if e.Seq > start && e.Seq <= end {
					page.events = append(page.events, e)
				}
			}
		}
		return page
	}
}

func (m *model) applyHistoryPage(page historyPage) {
	if page.initial && page.epoch != 0 && page.epoch != m.watchEpoch {
		return
	}
	if !page.initial && page.end != m.historyStart[page.id] {
		return
	}
	delete(m.historyLoading, page.id)
	if page.err != nil {
		m.notice = "History: " + page.err.Error()
		return
	}
	if m.historyStart == nil {
		m.historyStart = map[string]uint64{}
	}
	m.historyStart[page.id] = page.start
	if page.initial {
		last := page.start
		for _, e := range page.events {
			last = max(last, e.Seq)
		}
		// A final event from a canceled watcher may already be queued. Do not
		// drop it while leaving its replay cursor ahead of the loaded tail.
		for _, e := range m.events[page.id] {
			if e.Seq > last {
				page.events = append(page.events, e)
			}
		}
		m.events[page.id] = page.events
		for _, e := range page.events {
			m.cursor[page.id] = max(m.cursor[page.id], e.Seq)
		}
	} else {
		m.events[page.id] = append(page.events, m.events[page.id]...)
	}
	if s := m.current(); s != nil && s.Id == page.id {
		height, offset := m.view.TotalLineCount(), m.view.YOffset
		m.render()
		if page.initial {
			m.view.GotoBottom()
		} else {
			// Preserve the previously visible line when older content is prepended.
			m.view.SetYOffset(offset + m.view.TotalLineCount() - height)
		}
	}
}
