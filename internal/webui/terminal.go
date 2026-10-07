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
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/lesomnus/cxz/resource"
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
	c.SetReadLimit(32768)
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
	ack := make(chan int)
	go func() {
		defer cancel()
		inputEnded := false
		for {
			kind, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			if kind == websocket.MessageBinary {
				if len(data) == 0 {
					return
				}
				if !inputEnded {
					err = upstream.Send(resource.ProjectTerminalRequest_builder{Input: data}.Build())
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
				if control.Ack > 0 && control.Columns == 0 && control.Rows == 0 && control.Ack <= 32768 {
					select {
					case ack <- control.Ack:
					case <-ctx.Done():
						return
					}
				} else if control.Ack == 0 && control.Columns >= 1 && control.Columns <= 500 && control.Rows >= 1 && control.Rows <= 100 {
					if !inputEnded {
						err = upstream.Send(resource.ProjectTerminalRequest_builder{Columns: &control.Columns, Rows: &control.Rows}.Build())
					}
				} else {
					return
				}
			}
			clear(data)
			// A shell can exit before its queued output has reached the browser.
			// Keep reading acknowledgements and drain replies after Send's EOF.
			if errors.Is(err, io.EOF) {
				inputEnded = true
				continue
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		frame, err := upstream.Recv()
		if err != nil {
			_ = wsjson.Write(ctx, c, terminalStatus{Error: status.Convert(err).Message()})
			_ = c.Close(websocket.StatusNormalClosure, "")
			return
		}
		writeCtx, stop := context.WithTimeout(ctx, 30*time.Second)
		if output := frame.GetOutput(); len(output) > 0 {
			err = c.Write(writeCtx, websocket.MessageBinary, output)
			// One bounded chunk in flight. Ack only after xterm parses it, so a
			// fast producer cannot grow browser buffers indefinitely.
			if err == nil {
				select {
				case n := <-ack:
					if n != len(output) {
						err = context.Canceled
					}
				case <-writeCtx.Done():
					err = writeCtx.Err()
				}
			}
		} else {
			err = wsjson.Write(writeCtx, c, terminalStatus{Ready: frame.GetReady(), Exited: frame.GetExited(), Error: frame.GetError()})
		}
		stop()
		if frame.GetExited() && err == nil {
			_ = c.Close(websocket.StatusNormalClosure, "")
		}
		if err != nil || frame.GetExited() {
			return
		}
	}
}

func uintPointer(v uint32) *uint32 { return &v }
