package tui

import (
	"context"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

const historyPageSize uint64 = 128

type caughtUp struct {
	id       string
	events   []*api.Event
	epoch    uint64
	prepared *historyPage
}

type historyPage struct {
	floor      uint64
	generation uint64
	started    time.Time
	id         string
	// rows are the conversation as the runtime reduced it; events are the
	// records the page carried beside them, for what a reader derives rather
	// than reads. See the two stores on model.
	rows           []*api.Event
	events         []*api.Event
	snapshotSeq    uint64
	hasOlder       bool
	start          uint64
	end            uint64
	epoch          uint64
	initial        bool
	err            error
	prepared       map[*api.Event]renderedResponse
	toolActivities map[toolActivityKey]cachedToolActivity
	toolBodies     map[toolRenderKey]string
	inputBodies    map[inputRenderKey]string
	summaries      map[summaryRenderKey]string
}

// Transcript pages backwards on its own: before_seq with the session's snapshot
// as the fence, and has_older says whether anything is behind it. The sequence
// window this used to compute -- 128 journal coordinates at a time, because the
// native API only counts forwards -- is not needed for rows.
func (m *model) loadOlderHistory() tea.Cmd {
	if cmd := m.requestNewerHistory(false); cmd != nil {
		return cmd
	}
	return m.requestOlderHistory(false)
}

func (m *model) requestOlderHistory(warm bool) tea.Cmd {
	s := m.current()
	if s == nil || m.projectView || m.accountView || m.historyStart[s.Id] <= m.historyWindow(s.Id).floor || m.historyLoading[s.Id] > 0 {
		return nil
	}
	if !warm && m.historyWindow(s.Id).detached && m.historyWindow(s.Id).direction > 0 {
		return nil
	}
	// Start several screens before the loaded edge, leaving time for remote I/O
	// and Markdown preparation while the user continues scrolling locally.
	if !warm && !m.historyOpening[s.Id] && m.view.YOffset > 6*m.view.Height {
		return nil
	}
	id, end := s.Id, m.historyStart[s.Id]
	generation := m.historyWindow(id).generation
	agent, width := s.Agent, max(1, m.view.Width)
	start := uint64(0)
	if end > historyPageSize {
		start = end - historyPageSize
	}
	if m.historyLoading == nil {
		m.historyLoading = map[string]uint64{}
	}
	m.historyLoading[id] = end
	snapshot := m.cursor[id]
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		reply, err := m.fetchTranscript(ctx, id, &api.TranscriptRequest{BeforeSeq: end + 1, SnapshotSeq: snapshot}, "older")
		page := historyPage{id: id, start: start, end: end, err: err, generation: generation}
		if reply != nil {
			page.rows, page.events = reply.Events, reply.Metadata
			page.snapshotSeq, page.hasOlder = reply.SnapshotSeq, reply.HasOlder
			page.floor = historyBatchFloor(reply.Events)
			if len(reply.Events) > 0 {
				page.start = reply.Events[0].Seq - 1
			} else {
				page.start = max(page.floor, 0)
			}
			if !reply.HasOlder {
				page.start = page.floor
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
	// Rows arrive reduced, so there is nothing to pair here any more: what this
	// does is the expensive part -- Markdown, syntax highlighting, wrapping --
	// off the UI goroutine, keyed by the row it drew.
	for _, e := range page.rows {
		if err := ctx.Err(); err != nil {
			page.err = err
			return page
		}
		switch {
		case e.Kind == "input":
			prepared.cachedInput(&api.Session{Agent: agent}, e, width)
		case e.Kind == "assistant":
			page.prepared[e] = renderResponse(agent, e.Text, width)
			prepared.cachedSummary(e, width)
		case e.GetToolSummary() != nil:
			prepared.cachedToolBody(agent, e, width, e.GetToolSummary().GetState(), false)
		}
	}
	page.toolBodies = prepared.renderedTools
	page.inputBodies, page.summaries = prepared.renderedInputs, prepared.renderedSummaries
	m.debugRecorder.Add(debugEvent{Kind: "history_prepare", Duration: time.Since(started).Microseconds(), Count: len(page.prepared) + len(page.toolBodies)})
	return page
}

func (m *model) applyHistoryPage(page historyPage) bool {
	if !page.initial && page.generation != 0 && page.generation != m.historyWindow(page.id).generation {
		return false
	}
	anchor := m.historyAnchor()
	if page.initial && !m.validSessionWatch(page.id, page.epoch) {
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
		m.showError("History: " + page.err.Error())
		return true
	}
	w := m.historyWindow(page.id)
	m.applyHistoryFloor(page.id, max(page.floor, historyBatchFloor(page.rows)))
	var visible []*api.Event
	for _, e := range page.rows {
		if e.Seq > w.floor {
			visible = append(visible, e)
		}
	}
	// A page's snapshot is the journal's own fence, which reaches past the last
	// row: the records between the two are bookkeeping, and the replay cursor
	// has to clear them or the live stream replays what the page already settled.
	if page.initial {
		m.cursor[page.id] = max(m.cursor[page.id], page.snapshotSeq)
		for _, e := range page.rows {
			m.cursor[page.id] = max(m.cursor[page.id], e.Seq)
		}
		w.detached = false
		w.generation++
		w.loading = false
		w.tail = m.cursor[page.id]
	}
	page.rows = visible
	m.mergePreparedHistory(page)
	// The records a page carries are not the conversation; they are the latest
	// of the kinds a reader derives from. Merged by sequence so an older page
	// cannot push out what a newer one already said.
	m.mergeRecords(page.id, page.events)
	if m.historyStart == nil {
		m.historyStart = map[string]uint64{}
	}
	m.historyStart[page.id] = max(page.start, w.floor)
	if page.initial {
		last := page.start
		for _, e := range page.rows {
			last = max(last, e.Seq)
		}
		// A final row from a canceled watcher may already be queued. Do not
		// drop it while leaving its replay cursor ahead of the loaded tail.
		for _, e := range m.rows[page.id] {
			if e.Seq > last {
				page.rows = append(page.rows, e)
			}
		}
		if m.rows == nil {
			m.rows = map[string][]*api.Event{}
		}
		m.rows[page.id] = page.rows
	} else {
		m.rows[page.id] = append(page.rows, m.rows[page.id]...)
	}
	keep := 0.0
	if !page.initial {
		keep = m.readerAnchor(page.id)
	}
	trimmed := m.limitHistory(page.id, !page.initial, keep)
	if s := m.current(); s != nil && s.Id == page.id {
		height, offset := m.view.TotalLineCount(), m.view.YOffset
		opening := m.historyOpening[page.id]
		m.render()
		if page.initial || opening {
			m.view.GotoBottom()
			if !page.started.IsZero() {
				m.debugRecorder.Add(debugEvent{Kind: "history_first_render", Duration: time.Since(page.started).Microseconds(), Count: len(page.rows)})
			}
		} else {
			// Preserve the previously visible line when older content is prepended.
			m.view.SetYOffset(offset + m.view.TotalLineCount() - height)
			if trimmed {
				m.restoreHistoryAnchor(anchor)
			}
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

// fetchTranscript asks for a page of the conversation: the rows a person reads,
// and beside them the records a reader derives from -- quota, the model catalog,
// the compact boundary. Paging rows rather than records is also what bounds a
// page by what is readable: a native page of 1024 records may hold three
// readable rows or three hundred, so a scrollback cannot be built on it.
func (m *model) fetchTranscript(ctx context.Context, id string, r *api.TranscriptRequest, purpose string) (*api.TranscriptReply, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	started := time.Now()
	r.SessionId = id
	page, err := m.client.Transcript(ctx, r)
	count := 0
	if page != nil {
		count = len(page.Events)
	}
	m.debugRecorder.Add(debugEvent{Kind: "transcript_rpc", Type: purpose, Duration: time.Since(started).Microseconds(), Count: count, Bytes: proto.Size(page), ErrorCode: status.Code(err).String()})
	return page, err
}

// Merge only on the UI goroutine. A resize while work was in flight invalidates
// size-dependent rows naturally through their cache keys.
func (m *model) mergePreparedHistory(page historyPage) {
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

	if m.renderedInputs == nil {
		m.renderedInputs = map[inputRenderKey]string{}
	}
	for key, body := range page.inputBodies {
		if key.width == max(1, m.view.Width) {
			m.renderedInputs[key] = body
		}
	}
	if m.renderedSummaries == nil {
		m.renderedSummaries = map[summaryRenderKey]string{}
	}
	for key, body := range page.summaries {
		if key.width == max(1, m.view.Width) {
			m.renderedSummaries[key] = body
		}
	}
}

// mergeRecords keeps the latest record of each sequence a page carried beside
// its rows. They are what quota, the model catalog and the context badge are
// derived from, and a page answers with the most recent of each kind rather
// than with everything -- see transcripthistory's metadata kinds.
func (m *model) mergeRecords(id string, records []*api.Event) {
	if len(records) == 0 {
		return
	}
	if m.events == nil {
		m.events = map[string][]*api.Event{}
	}
	at := map[uint64]int{}
	merged := append([]*api.Event(nil), m.events[id]...)
	for i, e := range merged {
		at[e.Seq] = i
	}
	for _, e := range records {
		if i, ok := at[e.Seq]; ok {
			merged[i] = e
			continue
		}
		at[e.Seq] = len(merged)
		merged = append(merged, e)
	}
	slices.SortStableFunc(merged, func(a, b *api.Event) int {
		switch {
		case a.Seq < b.Seq:
			return -1
		case a.Seq > b.Seq:
			return 1
		}
		return 0
	})
	m.events[id] = merged
}

// liveRow is one row of the conversation as it is published. A row arrives
// again when a later record corrects it -- a response gains its turn's metrics,
// a tool completes -- and replacing by sequence is the whole of handling that.
type liveRow struct {
	id       string
	row      *api.Event
	updated  uint64
	epoch    uint64
	prepared *historyPage
}

// receiveRow puts a published row into the display list, replacing the version
// it corrects. A row's sequence is its identity: the runtime republishes the
// same sequence when a later record changes what that row says, and the later
// write wins.
func (m *model) receiveRow(v liveRow) {
	if m.rowCursor == nil {
		m.rowCursor = map[string]uint64{}
	}
	if v.updated > m.rowCursor[v.id] {
		m.rowCursor[v.id] = v.updated
	}
	w := m.historyWindow(v.id)
	if v.row.Seq <= w.floor {
		return
	}
	if m.rows == nil {
		m.rows = map[string][]*api.Event{}
	}
	rows := m.rows[v.id]
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].Seq == v.row.Seq {
			rows[i] = v.row
			return
		}
		if rows[i].Seq < v.row.Seq {
			break
		}
	}
	// A reader browsing older pages is not dragged to the tail, but the row is
	// still theirs to scroll to: the window's own bookkeeping decides what is
	// evicted, and appending past the loaded edge would leave a gap.
	if w.detached {
		return
	}
	at := len(rows)
	for at > 0 && rows[at-1].Seq > v.row.Seq {
		at--
	}
	rows = append(rows, nil)
	copy(rows[at+1:], rows[at:])
	rows[at] = v.row
	m.rows[v.id] = rows
	if !w.detached {
		w.tail = max(w.tail, v.row.Seq)
	}
	m.limitHistory(v.id, false, 0)
}

// liveRows is a coalesced batch of published rows, which is one change to what
// is on screen however many rows it carries.
type liveRows struct {
	id       string
	rows     []*api.Event
	updated  uint64
	epoch    uint64
	prepared *historyPage
}
