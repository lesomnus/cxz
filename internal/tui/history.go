package tui

import (
	"context"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

const historyPageSize uint64 = 128

type caughtUp struct {
	id     string
	events []*api.Event
	epoch  uint64
}

type historyPage struct {
	started        time.Time
	id             string
	events         []*api.Event
	start          uint64
	end            uint64
	epoch          uint64
	initial        bool
	err            error
	prepared       map[*api.Event]renderedResponse
	toolActivities map[toolActivityKey]cachedToolActivity
	toolBodies     map[toolRenderKey]string
}

// Journal sequence numbers are contiguous. The existing ascending History API
// returns at most 128 events, so a sequence window also provides reverse paging.
func (m *model) loadOlderHistory() tea.Cmd {
	return m.requestOlderHistory(false)
}

func (m *model) requestOlderHistory(warm bool) tea.Cmd {
	s := m.current()
	if s == nil || m.projectView || m.accountView || m.historyStart[s.Id] == 0 || m.historyLoading[s.Id] > 0 {
		return nil
	}
	// Start several screens before the loaded edge, leaving time for remote I/O
	// and Markdown preparation while the user continues scrolling locally.
	if !warm && !m.historyOpening[s.Id] && m.view.YOffset > 6*m.view.Height {
		return nil
	}
	id, end := s.Id, m.historyStart[s.Id]
	agent, width := s.Agent, max(1, m.view.Width)
	start := uint64(0)
	if end > historyPageSize {
		start = end - historyPageSize
	}
	if m.historyLoading == nil {
		m.historyLoading = map[string]uint64{}
	}
	m.historyLoading[id] = end
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		batch, err := m.fetchHistory(ctx, id, start, "older")
		page := historyPage{id: id, start: start, end: end, err: err}
		if batch != nil {
			for _, e := range batch.Events {
				if e.Seq > start && e.Seq <= end {
					page.events = append(page.events, e)
				}
			}
		}
		return m.prepareHistoryPage(ctx, page, agent, width)
	}
}

// Run on the fetch goroutine with captured session/width values. Publish the
// completed cache through the message; only the UI goroutine merges it.
func (m *model) prepareHistoryPage(ctx context.Context, page historyPage, agent string, width int) historyPage {
	if page.err != nil {
		return page
	}
	started := time.Now()
	page.prepared = map[*api.Event]renderedResponse{}
	prepared := &model{}
	results := map[string]*api.Event{}
	for _, e := range page.events {
		if e.Kind == "tool_result" && e.RequestId != "" {
			results[e.RunId+"/"+e.RequestId] = e
		}
	}
	for _, e := range page.events {
		if err := ctx.Err(); err != nil {
			page.err = err
			return page
		}
		if e.Kind == "assistant" {
			page.prepared[e] = renderResponse(agent, e.Text, width)
		}
		if e.Kind == "tool_call" && !question(e) {
			activity, ok := prepared.cachedToolView(agent, e)
			if !ok {
				continue
			}
			result := results[e.RunId+"/"+e.RequestId]
			if result != nil && agent == "codex" {
				if final, ok := prepared.cachedToolView(agent, result); ok {
					activity = final
				}
			}
			prepared.cachedToolBody(agent, e, activity, result, width, toolInitialState(agent, e), false)
		}
	}
	page.toolActivities, page.toolBodies = prepared.toolActivities, prepared.renderedTools
	m.debugRecorder.Add(debugEvent{Kind: "history_prepare", Duration: time.Since(started).Microseconds(), Count: len(page.prepared) + len(page.toolBodies)})
	return page
}

func (m *model) applyHistoryPage(page historyPage) bool {
	if page.initial && page.epoch != 0 && page.epoch != m.watchEpoch {
		return false
	}
	if !page.initial && m.historyLoading[page.id] == page.end {
		delete(m.historyLoading, page.id)
	}
	if !page.initial && page.end != m.historyStart[page.id] {
		return false
	}
	if page.err != nil {
		delete(m.historyOpening, page.id)
		if m.historyShimmer != nil && m.historyShimmer.id == page.id {
			m.historyShimmer = nil
		}
		m.notice = "History: " + page.err.Error()
		return true
	}
	if m.renderedResponses == nil {
		m.renderedResponses = map[*api.Event]renderedResponse{}
	}
	for e, prepared := range page.prepared {
		m.renderedResponses[e] = prepared
	}
	if m.toolActivities == nil {
		m.toolActivities = map[toolActivityKey]cachedToolActivity{}
	}
	for key, activity := range page.toolActivities {
		m.toolActivities[key] = activity
	}
	if m.renderedTools == nil {
		m.renderedTools = map[toolRenderKey]string{}
	}
	for key, body := range page.toolBodies {
		if key.width == max(1, m.view.Width) {
			m.renderedTools[key] = body
		}
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
		opening := m.historyOpening[page.id]
		m.render()
		if page.initial || opening {
			m.view.GotoBottom()
			if !page.started.IsZero() {
				m.debugRecorder.Add(debugEvent{Kind: "history_first_render", Duration: time.Since(page.started).Microseconds(), Count: len(page.events)})
			}
		} else {
			// Preserve the previously visible line when older content is prepended.
			m.view.SetYOffset(offset + m.view.TotalLineCount() - height)
		}
	}
	return true
}

// Bytes is the encoded protobuf payload size, not SSH/TCP traffic or compression.
func (m *model) fetchHistory(ctx context.Context, id string, after uint64, purpose string) (*api.EventBatch, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	started := time.Now()
	batch, err := m.client.History(ctx, &api.WatchRequest{SessionId: id, AfterSeq: after})
	count := 0
	if batch != nil {
		count = len(batch.Events)
	}
	m.debugRecorder.Add(debugEvent{Kind: "history_rpc", Type: purpose, Duration: time.Since(started).Microseconds(), Count: count, Bytes: proto.Size(batch), ErrorCode: status.Code(err).String()})
	return batch, err
}
