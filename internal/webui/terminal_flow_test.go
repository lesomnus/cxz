package webui

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

func TestTerminalWindowCreditAndCancellation(t *testing.T) {
	w := newTerminalWindow()
	for range terminalOutputHigh / terminalFrameBytes {
		if err := w.reserve(t.Context(), terminalFrameBytes); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []int{0, -1, terminalOutputHigh + 1} {
		if w.acknowledge(n) == nil {
			t.Fatal("accepted invalid credit", n)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := w.reserve(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("window did not pause", err)
	}
	if err := w.acknowledge(terminalFrameBytes); err != nil {
		t.Fatal(err)
	}
	ctx2, cancel2 := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel2()
	if err := w.reserve(ctx2, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("resumed above low watermark", err)
	}
	if err := w.acknowledge(terminalOutputHigh - terminalFrameBytes - terminalOutputLow); err != nil {
		t.Fatal(err)
	}
	if err := w.reserve(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	cancelled, stop := context.WithCancel(t.Context())
	stop()
	if !errors.Is(w.drain(cancelled), context.Canceled) || !errors.Is(w.reserve(cancelled, 1), context.Canceled) {
		t.Fatal("cancelled wait did not exit")
	}
	done := make(chan error, 1)
	go func() { done <- w.drain(t.Context()) }()
	if err := w.acknowledge(terminalOutputLow + 1); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("drain did not wake")
	}
	if w.acknowledge(1) == nil {
		t.Fatal("accepted duplicate credit")
	}
}

func TestTerminalWindowStalledTailTimeout(t *testing.T) {
	w := newTerminalWindow()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.watch(ctx, 20*time.Millisecond) }()
	// Idle connections do not time out; only outstanding output does.
	select {
	case err := <-done:
		t.Fatal("idle terminal expired", err)
	case <-time.After(40 * time.Millisecond):
	}
	if err := w.reserve(ctx, 1); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stalled small tail did not expire")
	}
}

type flowClient struct {
	resource.ProjectServiceClient
	stream *flowStream
}

func (c flowClient) Terminal(ctx context.Context, _ ...grpc.CallOption) (grpc.BidiStreamingClient[resource.ProjectTerminalRequest, resource.ProjectTerminalReply], error) {
	c.stream.ctx = ctx
	return c.stream, nil
}

type flowStream struct {
	grpc.ClientStream
	ctx          context.Context
	frames       chan *resource.ProjectTerminalReply
	inputStarted chan struct{}
}

func (s *flowStream) Send(r *resource.ProjectTerminalRequest) error {
	if r.HasInput() {
		select {
		case s.inputStarted <- struct{}{}:
		default:
		}
		<-s.ctx.Done() // Simulate a PTY that cannot consume input while output is blocked.
		return s.ctx.Err()
	}
	return nil
}
func (s *flowStream) Recv() (*resource.ProjectTerminalReply, error) {
	select {
	case r := <-s.frames:
		return r, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}
func openFlowFixture(t *testing.T, s *flowStream) (*websocket.Conn, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	server := httptest.NewServer(&terminalProxy{client: flowClient{stream: s}})
	t.Cleanup(server.Close)
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/terminal/project?columns=80&rows=20", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {server.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c, ctx
}

func TestTerminalPipelineWindowAndACKDuringBlockedInput(t *testing.T) {
	s := &flowStream{frames: make(chan *resource.ProjectTerminalReply, 24), inputStarted: make(chan struct{}, 1)}
	var want []byte
	for range 20 {
		// UTF-8 crosses frame boundaries; the proxy preserves bytes verbatim.
		data := bytes.Repeat([]byte("한글!"), 4682)
		data = data[:terminalFrameBytes]
		want = append(want, data...)
		s.frames <- resource.ProjectTerminalReply_builder{Output: data}.Build()
	}
	s.frames <- resource.ProjectTerminalReply_builder{Exited: boolPointer(true)}.Build()
	c, ctx := openFlowFixture(t, s)
	if err := c.Write(ctx, websocket.MessageBinary, []byte("blocked input")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.inputStarted:
	case <-ctx.Done():
		t.Fatal("input did not block")
	}
	type read struct {
		kind websocket.MessageType
		data []byte
		err  error
	}
	reads := make(chan read, 32)
	go func() {
		for {
			kind, data, err := c.Read(ctx)
			reads <- read{kind, data, err}
			if err != nil {
				return
			}
		}
	}()
	var got []byte
	for len(got) < terminalOutputHigh {
		r := <-reads
		if r.err != nil || r.kind != websocket.MessageBinary {
			t.Fatal("early status", r.err, string(r.data))
		}
		got = append(got, r.data...)
	}
	select {
	case r := <-reads:
		t.Fatal("exceeded unacknowledged window", len(r.data), r.err)
	case <-time.After(30 * time.Millisecond):
	}
	if err := wsjson.Write(ctx, c, terminalControl{Ack: terminalOutputHigh}); err != nil {
		t.Fatal(err)
	}
	for len(got) < len(want) {
		r := <-reads
		if r.err != nil || r.kind != websocket.MessageBinary {
			t.Fatal("early status", r.err, string(r.data))
		}
		got = append(got, r.data...)
		if err := wsjson.Write(ctx, c, terminalControl{Ack: len(r.data)}); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(got, want) {
		t.Fatal("output bytes changed")
	}
	r := <-reads
	if r.err != nil || r.kind != websocket.MessageText || !bytes.Contains(r.data, []byte(`"exited":true`)) {
		t.Fatal("missing drained exit", r.err, string(r.data))
	}
}

func TestTerminalCoalescesSmallOutputAndDrainsTail(t *testing.T) {
	s := &flowStream{frames: make(chan *resource.ProjectTerminalReply, 1001)}
	want := bytes.Repeat([]byte("abc"), 1000)
	for range 1000 {
		s.frames <- resource.ProjectTerminalReply_builder{Output: []byte("abc")}.Build()
	}
	s.frames <- resource.ProjectTerminalReply_builder{Exited: boolPointer(true)}.Build()
	c, ctx := openFlowFixture(t, s)
	var got []byte
	frames := 0
	for len(got) < len(want) {
		kind, data, err := c.Read(ctx)
		if err != nil || kind != websocket.MessageBinary {
			t.Fatal(err, string(data))
		}
		frames++
		got = append(got, data...)
		if err := wsjson.Write(ctx, c, terminalControl{Ack: len(data)}); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(got, want) || frames >= 100 {
		t.Fatal("small output not preserved/coalesced", frames)
	}
	var end terminalStatus
	if err := wsjson.Read(ctx, c, &end); err != nil || !end.Exited {
		t.Fatal("tail did not drain", err, end)
	}
	t.Logf("1000 upstream output fragments coalesced into %d WebSocket frames", frames)
}

func TestTerminalRejectsForgedACK(t *testing.T) {
	s := &flowStream{frames: make(chan *resource.ProjectTerminalReply, 1)}
	s.frames <- resource.ProjectTerminalReply_builder{Ready: boolPointer(true)}.Build()
	c, ctx := openFlowFixture(t, s)
	var ready terminalStatus
	if err := wsjson.Read(ctx, c, &ready); err != nil || !ready.Ready {
		t.Fatal(err, ready)
	}
	if err := wsjson.Write(ctx, c, terminalControl{Ack: 1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Read(ctx); err == nil {
		t.Fatal("accepted acknowledgement for unsent bytes")
	}
}
