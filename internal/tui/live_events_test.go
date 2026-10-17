package tui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type channelEvents struct {
	ctx    context.Context
	events chan streamEvent
}

func (s channelEvents) Recv() (*api.Event, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case next, ok := <-s.events:
		if !ok {
			return nil, io.EOF
		}
		return next.event, next.err
	}
}

func TestLiveEventsFlushQuietStreamAndCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	in := make(chan streamEvent, 1)
	out := make(chan []*api.Event, 1)
	done := make(chan error, 1)
	go func() {
		done <- collectLiveEvents(ctx, channelEvents{ctx, in}, func(events []*api.Event) { out <- events })
	}()
	e := &api.Event{Seq: 1, Kind: "assistant", Text: "last output before silence"}
	in <- streamEvent{event: e}
	select {
	case batch := <-out:
		if len(batch) != 1 || batch[0] != e {
			t.Fatal(batch)
		}
	case <-ctx.Done():
		t.Fatal("quiet stream never flushed")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled watcher kept collecting")
	}
}

func TestLiveEventsBoundedOrderedAndFlushedBeforeError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	in := make(chan streamEvent, 300)
	for i := 1; i <= 300; i++ {
		in <- streamEvent{event: &api.Event{Seq: uint64(i), Kind: "raw"}}
	}
	close(in)
	var batches [][]*api.Event
	err := collectLiveEvents(ctx, channelEvents{ctx, in}, func(events []*api.Event) { batches = append(batches, events) })
	if err != io.EOF {
		t.Fatal(err)
	}
	seq := uint64(0)
	for _, batch := range batches {
		if len(batch) > liveBatchEvents {
			t.Fatal("unbounded batch", len(batch))
		}
		for _, e := range batch {
			seq++
			if e.Seq != seq {
				t.Fatal("event lost or reordered", seq, e.Seq)
			}
		}
	}
	if seq != 300 || len(batches) >= 300 {
		t.Fatal("stream was not coalesced", seq, len(batches))
	}
	// One unusually large event flushes immediately, without waiting for EOF.
	in = make(chan streamEvent, 2)
	in <- streamEvent{event: &api.Event{Seq: 1, Text: strings.Repeat("x", liveBatchBytes)}}
	want := errors.New("disconnected")
	in <- streamEvent{err: want}
	batches = nil
	if err := collectLiveEvents(ctx, channelEvents{ctx, in}, func(events []*api.Event) { batches = append(batches, events) }); err != want || len(batches) != 1 || len(batches[0]) != 1 {
		t.Fatal("last batch lost before disconnect", err)
	}
}

func TestPreparedLiveBatchKeepsDraftAndRendersOnce(t *testing.T) {
	m := conversationModel()
	m.watchEpoch = 2
	m.cursor["s"] = 0
	m.debugRecorder = &debugRecorder{}
	m.debugRecorder.Start()
	events := []*api.Event{
		{Seq: 1, RunId: "run", Kind: "input", Text: "task", TimeMs: 10},
		{Seq: 2, RunId: "run", Kind: "assistant", Text: "```go\npackage main\nfunc main() {}\n```"},
		{Seq: 3, RunId: "run", Kind: "turn_end", Text: "completed", TimeMs: 1010, Payload: []byte(`{"usage":{"input_tokens":12}}`)},
	}
	prepared := make(chan historyPage, 1)
	entered, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		close(entered)
		<-release // Model a slow syntax highlighter on the watch worker.
		prepared <- m.prepareHistoryPage(ctx, historyPage{rows: rowsOf(events)}, "claude", m.view.Width)
	}()
	<-entered
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("draft while preparing")})
	if m.input.Value() != "draft while preparing" || len(m.renderedResponses) != 0 {
		t.Fatal("preparation changed UI before delivery")
	}
	close(release)
	page := <-prepared
	if len(page.prepared) != 1 || len(page.inputBodies) != 1 || len(page.summaries) != 1 {
		t.Fatal("live rows not prepared")
	}
	// Records settle the replay cursor; rows carry the conversation. Both come
	// from the same batch of journal writes, on the two streams.
	m.Update(caughtUp{id: "s", events: events, epoch: 2})
	m.Update(liveRows{id: "s", rows: page.rows, updated: 3, epoch: 2, prepared: &page})
	if m.cursor["s"] != 3 || len(m.events["s"]) != 3 || m.input.Value() != "draft while preparing" {
		t.Fatal("live batch lost data or draft")
	}
	answer := page.rows[1]
	if m.renderedResponses[answer].body != page.prepared[answer].body || !strings.Contains(m.view.View(), "CLAUDE") {
		t.Fatal("prepared response not displayed")
	}
	m.Update(caughtUp{id: "s", events: []*api.Event{{Seq: 4, Kind: "raw"}}, epoch: 2})
	m.Update(caughtUp{id: "s", events: []*api.Event{{Seq: 5, Kind: "assistant", Text: "stale"}}, epoch: 1})
	if m.cursor["s"] != 4 || len(m.events["s"]) != 4 {
		t.Fatal("raw event lost or old watcher accepted")
	}
	renders := 0
	for _, event := range m.debugRecorder.Stop().Events {
		if event.Kind == "transcript_render" {
			renders++
		}
	}
	if renders != 1 {
		t.Fatal("rebuilt transcript per event or for raw-only batch", renders)
	}
}

type liveWatchClient struct {
	api.SessionsClient
	events chan streamEvent
	rows   chan streamRow
}

type liveRowStream struct {
	api.Sessions_RowsClient
	ctx  context.Context
	rows chan streamRow
}

func (s *liveRowStream) Recv() (*api.Row, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case next, ok := <-s.rows:
		if !ok {
			return nil, io.EOF
		}
		return next.row, next.err
	}
}

func (c liveWatchClient) Rows(ctx context.Context, _ *api.RowsRequest, _ ...grpc.CallOption) (api.Sessions_RowsClient, error) {
	return &liveRowStream{ctx: ctx, rows: c.rows}, nil
}

type liveWatchStream struct {
	api.Sessions_WatchClient
	channelEvents
}

func (c liveWatchClient) Watch(ctx context.Context, _ *api.WatchRequest, _ ...grpc.CallOption) (api.Sessions_WatchClient, error) {
	return &liveWatchStream{channelEvents: channelEvents{ctx, c.events}}, nil
}
func (s *liveWatchStream) Recv() (*api.Event, error) { return s.channelEvents.Recv() }

type liveWatchProbe struct{ messages chan liveRows }

func (*liveWatchProbe) Init() tea.Cmd { return nil }
func (*liveWatchProbe) View() string  { return "" }
func (p *liveWatchProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if batch, ok := msg.(liveRows); ok {
		p.messages <- batch
	}
	return p, nil
}

func TestWatchPreparesLiveResponseAtCurrentWidth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	m := conversationModel()
	m.ctx = ctx
	client := liveWatchClient{events: make(chan streamEvent, 1), rows: make(chan streamRow, 1)}
	m.client = client
	probe := &liveWatchProbe{messages: make(chan liveRows, 1)}
	program := tea.NewProgram(probe, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	m.program = program
	done := make(chan struct{})
	go func() { program.Run(); close(done) }()
	defer func() { cancel(); <-done }()
	m.watch()
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 30})
	// The published row, which is what the transcript draws. Preparing it is
	// the expensive part and belongs off the UI goroutine at the live width.
	e := &api.Event{Seq: 1, RunId: "run", Kind: "assistant", Text: "```go\npackage main\n```"}
	client.rows <- streamRow{row: &api.Row{Event: e, Updated: 1}}
	select {
	case batch := <-probe.messages:
		if batch.id != "s" || batch.epoch != m.watchEpoch || len(batch.rows) != 1 || batch.prepared == nil || batch.prepared.prepared[e].width != m.view.Width {
			t.Fatal("the row stream did not prepare a batch at the resized width", batch)
		}
	case <-ctx.Done():
		t.Fatal("watch failed to publish live response")
	}
}

// A record stream that stops saying it is alive is reported, so the watcher
// reconnects. The TUI depends on this stream for everything the transcript
// cannot show -- execution state, quota, pending approvals -- so a stall that
// went unnoticed would freeze all of it while the conversation kept arriving
// on the row stream. See cxz#191.
func TestQuietRecordStreamIsReportedOnceItHasSaidItIsAlive(t *testing.T) {
	was := liveStallInterval
	liveStallInterval = 40 * time.Millisecond
	t.Cleanup(func() { liveStallInterval = was })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	in := make(chan streamEvent, 1)
	out := make(chan []*api.Event, 1)
	done := make(chan error, 1)
	go func() {
		done <- collectLiveEvents(ctx, channelEvents{ctx, in}, func(events []*api.Event) { out <- events })
	}()
	// A heartbeat is not a batch: it carries no record, and a reader that
	// rendered it would rebuild the transcript for nothing.
	in <- streamEvent{event: &api.Event{Heartbeat: true}}
	select {
	case batch := <-out:
		t.Fatal("a heartbeat was delivered as records", batch)
	case err := <-done:
		if !errors.Is(err, errStreamStalled) {
			t.Fatal("the quiet stream ended for another reason", err)
		}
	case <-ctx.Done():
		t.Fatal("the quiet stream was never reported")
	}
}

// A stream that never says it is alive is waited on indefinitely: that is an
// older runtime being legitimately silent, and restarting it every interval
// would be this client fighting a healthy session.
func TestRecordStreamWithoutHeartbeatsIsNotTimedOut(t *testing.T) {
	was := liveStallInterval
	liveStallInterval = 30 * time.Millisecond
	t.Cleanup(func() { liveStallInterval = was })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	in := make(chan streamEvent, 1)
	done := make(chan error, 1)
	go func() {
		done <- collectLiveEvents(ctx, channelEvents{ctx, in}, func([]*api.Event) {})
	}()
	in <- streamEvent{event: &api.Event{Seq: 1, Kind: "assistant", Text: "the only thing it will ever say"}}
	select {
	case err := <-done:
		t.Fatal("a silent stream with no heartbeats was ended", err)
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
