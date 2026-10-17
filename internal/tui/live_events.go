package tui

import (
	"context"
	"errors"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/transcripthistory"
)

const liveBatchInterval = 50 * time.Millisecond

// Two missed heartbeats. The interval itself is the runtime's, shared with the
// row stream so the two cannot drift on what silence means.
var liveStallInterval = 2 * transcripthistory.Heartbeat

// errStreamStalled ends a stream that stopped saying it was alive. The watcher
// treats it like any other stream failure, which is to reconnect.
var errStreamStalled = errors.New("the session stream stopped responding")

const liveBatchEvents = 128
const liveBatchBytes = 512 * 1024

type eventReceiver interface{ Recv() (*api.Event, error) }
type streamEvent struct {
	event *api.Event
	err   error
}

// The reader has one queued item; preparation or a busy UI applies bounded
// backpressure. Every journal event, including raw events, stays in order.
// A quiet stream still flushes its final partial batch without another event.
//
// A heartbeat frame is not a record: it is the runtime saying the stream is
// alive with nothing to send. It never enters a batch -- it carries no
// sequence, and a reader that advanced its cursor by it would rewind -- and
// missing two of them in a row ends the stream so the watcher reconnects.
// Armed only once one has been seen, because a runtime that predates them is
// legitimately silent and timing it out would restart a healthy stream.
func collectLiveEvents(ctx context.Context, stream eventReceiver, emit func([]*api.Event)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	incoming := make(chan streamEvent, 1)
	go func() {
		for {
			e, err := stream.Recv()
			select {
			case incoming <- streamEvent{e, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	timer := time.NewTimer(liveBatchInterval)
	defer timer.Stop()
	timer.Stop()
	stall := time.NewTimer(liveStallInterval)
	defer stall.Stop()
	stall.Stop()
	var quiet <-chan time.Time
	var due <-chan time.Time
	var batch []*api.Event
	bytes := 0
	flush := func() {
		if len(batch) > 0 && ctx.Err() == nil {
			emit(batch)
		}
		batch, bytes = nil, 0
		timer.Stop()
		due = nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-quiet:
			flush()
			return errStreamStalled
		case <-due:
			flush()
		case next := <-incoming:
			if next.err != nil {
				flush()
				return next.err
			}
			if quiet != nil || next.event.GetHeartbeat() {
				stall.Reset(liveStallInterval)
				quiet = stall.C
			}
			if next.event.GetHeartbeat() {
				continue
			}
			if len(batch) == 0 {
				timer.Reset(liveBatchInterval)
				due = timer.C
			}
			batch = append(batch, next.event)
			bytes += len(next.event.Text) + len(next.event.Payload)
			if len(batch) >= liveBatchEvents || bytes >= liveBatchBytes {
				flush()
			}
		}
	}
}

type rowReceiver interface{ Recv() (*api.Row, error) }
type streamRow struct {
	row *api.Row
	err error
}

// collectLiveRows coalesces published rows the way collectLiveEvents does for
// records, and for the same reason: a burst of rows is one change to what is on
// screen, and rebuilding the transcript per row puts the whole window back on
// the UI goroutine once per row.
//
// A row with no event is the runtime saying the stream is alive with nothing to
// send. It carries the cursor forward and nothing else, so it does not open a
// batch -- a heartbeat must not cost a rebuild.
func collectLiveRows(ctx context.Context, stream rowReceiver, emit func([]*api.Row)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	incoming := make(chan streamRow, 1)
	go func() {
		for {
			r, err := stream.Recv()
			select {
			case incoming <- streamRow{r, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	timer := time.NewTimer(liveBatchInterval)
	defer timer.Stop()
	timer.Stop()
	var due <-chan time.Time
	var batch []*api.Row
	bytes := 0
	flush := func() {
		if len(batch) > 0 && ctx.Err() == nil {
			emit(batch)
		}
		batch, bytes = nil, 0
		timer.Stop()
		due = nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-due:
			flush()
		case next := <-incoming:
			if next.err != nil {
				flush()
				return next.err
			}
			if next.row.Event == nil {
				if len(batch) == 0 {
					emit([]*api.Row{next.row})
				} else {
					batch = append(batch, next.row)
				}
				continue
			}
			if len(batch) == 0 {
				timer.Reset(liveBatchInterval)
				due = timer.C
			}
			batch = append(batch, next.row)
			bytes += len(next.row.Event.Text) + len(next.row.Event.Payload)
			if len(batch) >= liveBatchEvents || bytes >= liveBatchBytes {
				flush()
			}
		}
	}
}
