//go:build linux

package tui

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

type inlineReadyWriter struct {
	bytes.Buffer
	ready chan struct{}
	once  sync.Once
}

func (w *inlineReadyWriter) Write(b []byte) (int, error) {
	n, err := w.Buffer.Write(b)
	if strings.Contains(w.Buffer.String(), "[ Trust ]") {
		w.once.Do(func() { close(w.ready) })
	}
	return n, err
}
func TestInlineErrorPTYDoesNotUseFullscreen(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Skip(err)
	}
	defer master.Close()
	defer slave.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	out := &inlineReadyWriter{ready: make(chan struct{})}
	type result struct {
		yes bool
		err error
	}
	done := make(chan result, 1)
	go func() {
		yes, err := InlineError(ctx, slave, out, "privileged container requested", true)
		done <- result{yes, err}
	}()
	select {
	case <-out.ready:
	case <-ctx.Done():
		t.Fatal("inline prompt did not render")
	}
	// Pasted navigation/Enter must not accept; deliberate Right+Enter accepts.
	if _, err = master.Write([]byte("\x1b[200~\x1b[C\r\x1b[201~\x1b[C\r")); err != nil {
		t.Fatal(err)
	}
	r := <-done
	if r.err != nil || !r.yes {
		t.Fatal(r)
	}
	if strings.Contains(out.String(), "\x1b[?1049") {
		t.Fatal("used alternate screen")
	}
}
