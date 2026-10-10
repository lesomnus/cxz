package webui

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	terminalFrameBytes  = 32768
	terminalOutputHigh  = 256 << 10
	terminalOutputLow   = 64 << 10
	terminalInputLimit  = 1 << 20
	terminalInputFrames = 128
	terminalBatchDelay  = 5 * time.Millisecond
	terminalFlowTimeout = 30 * time.Second
)

// Credit is measured in bytes, independently of RPC/WebSocket chunk boundaries.
// Reserving before Write also handles an acknowledgement racing its return.
type terminalWindow struct {
	mu       sync.Mutex
	pending  int
	paused   bool
	progress time.Time
	changed  chan struct{}
}

func newTerminalWindow() *terminalWindow {
	return &terminalWindow{changed: make(chan struct{})}
}

func (w *terminalWindow) notify() {
	close(w.changed)
	w.changed = make(chan struct{})
}

func (w *terminalWindow) reserve(ctx context.Context, n int) error {
	if n <= 0 || n > terminalFrameBytes {
		return fmt.Errorf("invalid terminal output size")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		w.mu.Lock()
		if w.pending+n > terminalOutputHigh {
			w.paused = true
		}
		if w.paused && w.pending <= terminalOutputLow {
			w.paused = false
		}
		if !w.paused {
			if w.pending == 0 {
				w.progress = time.Now()
				w.notify()
			}
			w.pending += n
			w.mu.Unlock()
			return nil
		}
		changed := w.changed
		w.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (w *terminalWindow) acknowledge(n int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if n <= 0 || n > w.pending {
		return fmt.Errorf("invalid terminal acknowledgement")
	}
	w.pending -= n
	w.progress = time.Now()
	w.notify()
	return nil
}

func (w *terminalWindow) drain(ctx context.Context) error {
	for {
		w.mu.Lock()
		pending, changed := w.pending, w.changed
		w.mu.Unlock()
		if pending == 0 {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Also expire stalled clients when the last output was smaller than the window.
func (w *terminalWindow) watch(ctx context.Context, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		w.mu.Lock()
		pending, progress, changed := w.pending, w.progress, w.changed
		w.mu.Unlock()
		if pending == 0 {
			select {
			case <-changed:
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}
		remaining := timeout - time.Since(progress)
		if remaining <= 0 {
			return context.DeadlineExceeded
		}
		timer.Reset(remaining)
		select {
		case <-changed:
			timer.Stop()
		case <-timer.C:
			// Recheck under the mutex: an ACK can race the timer firing.
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
