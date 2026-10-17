package tui

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/api"
)

const liveBatchInterval = 50 * time.Millisecond
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
		case <-due:
			flush()
		case next := <-incoming:
			if next.err != nil {
				flush()
				return next.err
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
