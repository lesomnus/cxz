package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// Browser fetch cannot stream client input. This authenticated WebSocket only
// bridges the existing Manager Terminal RPC; it has no Docker or shell access.
type terminalProxy struct{ client resource.ProjectServiceClient }

var terminalProjectID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type terminalControl struct {
	Columns uint32 `json:"columns,omitempty"`
	Rows    uint32 `json:"rows,omitempty"`
	Ack     int    `json:"ack,omitempty"`
}
type terminalStatus struct {
	Ready  bool   `json:"ready,omitempty"`
	Exited bool   `json:"exited,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (p *terminalProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/terminal/")
	cols, ce := strconv.Atoi(r.URL.Query().Get("columns"))
	rows, re := strconv.Atoi(r.URL.Query().Get("rows"))
	// Require Origin even on GET upgrades. browserAuth has checked its exact value.
	if r.Method != http.MethodGet || r.Header.Get("Origin") == "" || !terminalProjectID.MatchString(id) || ce != nil || re != nil || cols < 1 || cols > 500 || rows < 1 || rows > 100 {
		http.Error(w, "invalid terminal connection", http.StatusBadRequest)
		return
	}
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(terminalFrameBytes)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	upstream, err := p.client.Terminal(ctx)
	if err == nil {
		err = upstream.Send(resource.ProjectTerminalRequest_builder{Ref: resource.ProjectRef_builder{RuntimeId: &id}.Build(), Columns: uintPointer(uint32(cols)), Rows: uintPointer(uint32(rows))}.Build())
	}
	if err != nil {
		_ = wsjson.Write(ctx, c, terminalStatus{Error: status.Convert(err).Message()})
		_ = c.Close(websocket.StatusNormalClosure, "")
		return
	}
	window := newTerminalWindow()
	go func() {
		if window.watch(ctx, terminalFlowTimeout) != nil {
			cancel()
		}
	}()
	// The socket reader must remain able to process ACKs while gRPC input is
	// blocked by a shell that is itself waiting for output to drain.
	input := make(chan *resource.ProjectTerminalRequest, terminalInputFrames)
	var inputBytes atomic.Int64
	go func() {
		defer cancel()
		ended := false
		for {
			select {
			case <-ctx.Done():
				return
			case request := <-input:
				n := len(request.GetInput())
				var err error
				if !ended {
					err = upstream.Send(request)
				}
				clear(request.GetInput())
				inputBytes.Add(-int64(n))
				if errors.Is(err, io.EOF) {
					ended = true
					continue
				}
				if err != nil {
					return
				}
			}
		}
	}()
	enqueue := func(request *resource.ProjectTerminalRequest) bool {
		n := int64(len(request.GetInput()))
		if inputBytes.Add(n) > terminalInputLimit {
			inputBytes.Add(-n)
			return false
		}
		select {
		case input <- request:
			return true
		default:
			inputBytes.Add(-n)
			return false
		}
	}
	go func() {
		defer cancel()
		for {
			kind, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			if kind == websocket.MessageBinary {
				if len(data) == 0 {
					return
				}
				if !enqueue(resource.ProjectTerminalRequest_builder{Input: data}.Build()) {
					return
				}
			} else {
				var control terminalControl
				decoder := json.NewDecoder(bytes.NewReader(data))
				decoder.DisallowUnknownFields()
				decodeErr := decoder.Decode(&control)
				endErr := decoder.Decode(&struct{}{})
				if decodeErr != nil || endErr != io.EOF {
					return
				}
				if control.Ack > 0 && control.Columns == 0 && control.Rows == 0 && control.Ack <= terminalOutputHigh {
					if window.acknowledge(control.Ack) != nil {
						return
					}
				} else if control.Ack == 0 && control.Columns >= 1 && control.Columns <= 500 && control.Rows >= 1 && control.Rows <= 100 {
					if !enqueue(resource.ProjectTerminalRequest_builder{Columns: &control.Columns, Rows: &control.Rows}.Build()) {
						return
					}
				} else {
					return
				}
				clear(data)
			}
		}
	}()
	p.forwardOutput(ctx, c, upstream, window)
}

func (*terminalProxy) forwardOutput(ctx context.Context, c *websocket.Conn, upstream grpc.BidiStreamingClient[resource.ProjectTerminalRequest, resource.ProjectTerminalReply], window *terminalWindow) {
	type reply struct {
		frame *resource.ProjectTerminalReply
		err   error
	}
	frames := make(chan reply, 1)
	go func() {
		for {
			frame, err := upstream.Recv()
			select {
			case frames <- reply{frame, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	batch := make([]byte, 0, terminalFrameBytes)
	timer := time.NewTimer(terminalBatchDelay)
	timer.Stop()
	defer timer.Stop()
	var tick <-chan time.Time
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		timer.Stop()
		tick = nil
		writeCtx, stop := context.WithTimeout(ctx, terminalFlowTimeout)
		defer stop()
		if err := window.reserve(writeCtx, len(batch)); err != nil {
			return err
		}
		if err := c.Write(writeCtx, websocket.MessageBinary, batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}
	finish := func() error {
		if err := flush(); err != nil {
			return err
		}
		drainCtx, stop := context.WithTimeout(ctx, terminalFlowTimeout)
		defer stop()
		return window.drain(drainCtx)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			if flush() != nil {
				return
			}
		case next := <-frames:
			if next.err != nil {
				if finish() == nil {
					writeCtx, stop := context.WithTimeout(ctx, terminalFlowTimeout)
					_ = wsjson.Write(writeCtx, c, terminalStatus{Error: status.Convert(next.err).Message()})
					stop()
				}
				return
			}
			frame := next.frame
			for output := frame.GetOutput(); len(output) > 0; {
				if len(batch) == 0 {
					timer.Reset(terminalBatchDelay)
					tick = timer.C
				}
				n := min(len(output), terminalFrameBytes-len(batch))
				batch = append(batch, output[:n]...)
				output = output[n:]
				if len(batch) == terminalFrameBytes && flush() != nil {
					return
				}
			}
			if frame.GetReady() || frame.GetExited() || frame.GetError() != "" {
				if flush() != nil {
					return
				}
				if (frame.GetExited() || frame.GetError() != "") && finish() != nil {
					return
				}
				writeCtx, stop := context.WithTimeout(ctx, terminalFlowTimeout)
				err := wsjson.Write(writeCtx, c, terminalStatus{Ready: frame.GetReady(), Exited: frame.GetExited(), Error: frame.GetError()})
				stop()
				if frame.GetExited() && err == nil {
					_ = c.Close(websocket.StatusNormalClosure, "")
				}
				if err != nil || frame.GetExited() || frame.GetError() != "" {
					return
				}
			}
		}
	}
}

func uintPointer(v uint32) *uint32 { return &v }
