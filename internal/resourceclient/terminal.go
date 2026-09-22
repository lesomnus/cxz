package resourceclient

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) OpenTerminal(ctx context.Context, project string, columns, rows int) (containerterm.Terminal, error) {
	ctx, cancel := context.WithCancel(ctx)
	columns, rows = containerterm.Dimensions(columns, rows)
	w, h := uint32(columns), uint32(rows)
	stream, err := c.projects.Terminal(ctx)
	if err == nil {
		err = stream.Send(resource.ProjectTerminalRequest_builder{Ref: pr(project), Columns: &w, Rows: &h}.Build())
		// Even when Send reports EOF, receive the server's actionable status.
		if err == nil || err == io.EOF {
			var ready *resource.ProjectTerminalReply
			ready, err = stream.Recv()
			if err == nil && (!ready.GetReady() || ready.GetExited() || len(ready.GetOutput()) != 0) {
				err = errors.New("invalid terminal ready response")
			}
		}
	}
	if err != nil {
		cancel()
		if status.Code(err) == codes.Unimplemented {
			return nil, errors.New("Update host manager: cxz install --recreate")
		}
		return nil, err
	}
	return &remoteTerminal{stream: stream, cancel: cancel}, nil
}

type remoteTerminal struct {
	stream   grpc.BidiStreamingClient[resource.ProjectTerminalRequest, resource.ProjectTerminalReply]
	cancel   context.CancelFunc
	writeMu  sync.Mutex
	pending  []byte
	finished bool
	err      error // Read and Wait share one reader goroutine.
}

func (t *remoteTerminal) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(t.pending) == 0 {
		if t.finished {
			return 0, io.EOF
		}
		msg, err := t.stream.Recv()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			t.err, t.finished = err, true
			return 0, err
		}
		if msg.GetReady() || (msg.GetExited() && len(msg.GetOutput()) != 0) || len(msg.GetOutput()) > 32768 {
			t.err, t.finished = errors.New("invalid terminal output frame"), true
			return 0, t.err
		}
		t.pending = msg.GetOutput()
		if msg.GetExited() {
			t.finished = true
			if msg.GetError() != "" {
				t.err = errors.New(msg.GetError())
			}
		}
	}
	n := copy(p, t.pending)
	t.pending = t.pending[n:]
	return n, nil
}

func (t *remoteTerminal) Write(p []byte) (int, error) {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	sent := 0
	for len(p) > 0 {
		n := min(len(p), 32768)
		if err := t.stream.Send(resource.ProjectTerminalRequest_builder{Input: append([]byte(nil), p[:n]...)}.Build()); err != nil {
			return sent, err
		}
		sent += n
		p = p[n:]
	}
	return sent, nil
}
func (t *remoteTerminal) Resize(columns, rows int) error {
	columns, rows = containerterm.Dimensions(columns, rows)
	w, h := uint32(columns), uint32(rows)
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return t.stream.Send(resource.ProjectTerminalRequest_builder{Columns: &w, Rows: &h}.Build())
}
func (t *remoteTerminal) Wait() error  { return t.err }
func (t *remoteTerminal) Close() error { t.cancel(); return nil }
