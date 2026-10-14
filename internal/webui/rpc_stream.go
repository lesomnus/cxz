package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

var streamRPCPath = regexp.MustCompile(`^/(cxz|payday)\.[A-Za-z0-9_.]+/[A-Za-z][A-Za-z0-9_]*$`)

// Carry the existing Connect HTTP response over a socket on HTTP/1.1 origins.
// Registration, dispatch, protobuf conversion and authorization stay in rpc.
type rpcStreamProxy struct{ rpc http.Handler }

func (p *rpcStreamProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/rpc-stream")
	if r.Method != http.MethodGet || r.Header.Get("Origin") == "" || r.URL.RawQuery != "" || !streamRPCPath.MatchString(path) {
		http.Error(w, "invalid RPC stream", http.StatusBadRequest)
		return
	}
	socket, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer socket.CloseNow()
	socket.SetReadLimit(8 << 20)
	initial, cancelInitial := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancelInitial()
	var frame struct {
		Headers map[string]string `json:"headers"`
	}
	kind, data, err := socket.Read(initial)
	if err != nil || kind != websocket.MessageText || len(data) > 65536 || json.Unmarshal(data, &frame) != nil {
		_ = socket.Close(websocket.StatusPolicyViolation, "invalid request headers")
		return
	}
	contentType := frame.Headers["content-type"]
	if contentType != "application/connect+json" && contentType != "application/connect+proto" {
		_ = socket.Close(websocket.StatusPolicyViolation, "invalid request content type")
		return
	}
	kind, data, err = socket.Read(initial)
	if err != nil || kind != websocket.MessageBinary {
		return
	}
	cancelInitial()
	ctx, cancel := context.WithCancel(socket.CloseRead(r.Context()))
	defer cancel()
	request := r.Clone(ctx)
	request.Method = http.MethodPost
	request.URL.Path = path
	request.RequestURI = path
	request.Header = make(http.Header)
	for name, value := range frame.Headers {
		lower := strings.ToLower(name)
		switch lower {
		case "host", "cookie", "authorization", "origin", "connection", "upgrade", "content-length", "transfer-encoding":
			continue
		}
		if strings.HasPrefix(lower, "sec-") || strings.HasPrefix(lower, "proxy-") {
			continue
		}
		request.Header.Set(name, value)
	}
	request.Body = io.NopCloser(bytes.NewReader(data))
	request.ContentLength = int64(len(data))
	response := &rpcStreamResponse{socket: socket, ctx: ctx, cancel: cancel, header: make(http.Header)}
	p.rpc.ServeHTTP(response, request)
	if ctx.Err() == nil {
		response.WriteHeader(http.StatusOK)
		_ = wsjson.Write(ctx, socket, map[string]bool{"done": true})
		_ = socket.Close(websocket.StatusNormalClosure, "")
	}
}

type rpcStreamResponse struct {
	socket      *websocket.Conn
	ctx         context.Context
	cancel      context.CancelFunc
	header      http.Header
	wroteHeader bool
	err         error
}

func (w *rpcStreamResponse) Header() http.Header { return w.header }
func (w *rpcStreamResponse) WriteHeader(status int) {
	if w.wroteHeader || status < 200 {
		return
	}
	w.wroteHeader = true
	headers := make(map[string]string, len(w.header))
	for name, values := range w.header {
		headers[name] = strings.Join(values, ", ")
	}
	w.err = wsjson.Write(w.ctx, w.socket, struct {
		Status  int               `json:"status"`
		Headers map[string]string `json:"headers"`
	}{status, headers})
	if w.err != nil {
		w.cancel()
	}
}
func (w *rpcStreamResponse) Write(body []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	if w.err != nil {
		return 0, w.err
	}
	if err := w.socket.Write(w.ctx, websocket.MessageBinary, body); err != nil {
		w.err = err
		w.cancel()
		return 0, err
	}
	return len(body), nil
}
func (w *rpcStreamResponse) Flush() { w.WriteHeader(http.StatusOK) }
