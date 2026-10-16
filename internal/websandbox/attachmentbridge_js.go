//go:build js && wasm

package websandbox

import (
	"bytes"
	"net/http"
	"strconv"
	"syscall/js"

	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/attachmenthttp"
)

// Message is the field the page's upload uses to address this, and the field
// this answers on. `@lesomnus/grpc-dgram` reads `drpc`, so the two protocols
// share the worker's message channel without either having to know the other.
const Message = "cxz-attachment"
const MessageResult = "cxz-attachment-result"

// ServeAttachments carries file bodies to the same raw HTTP handler the Manager
// serves, over the worker's message channel.
//
// The sandbox has no network, so there is nothing for the page to POST to: this
// is the one surface where "upload over HTTP" cannot mean HTTP/2 on a socket.
// What it does mean is that the request is an *http.Request and the answer comes
// from [attachmenthttp.Handler] -- the same parsing, the same limits, the same
// status codes, the same JSON. Nothing above this knows which transport carried
// it, which is the whole point of a design sandbox: a page that only ever ran
// against a different upload path has never run.
func ServeAttachments(u assets.Uploader) func() {
	h := attachmenthttp.Handler{Uploader: u}
	cb := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		data := args[0].Get("data")
		if data.Type() != js.TypeObject || data.Get("cxz").String() != Message {
			return nil
		}
		id, url := data.Get("id").String(), data.Get("url").String()
		// Copied before returning: a js.Value is only valid for this call, and
		// the upload runs on its own goroutine so a handler that blocks cannot
		// hold the event loop the page's other calls arrive on.
		view := js.Global().Get("Uint8Array").New(data.Get("body"))
		body := make([]byte, view.Get("byteLength").Int())
		js.CopyBytesToGo(body, view)
		go func() {
			request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				reply(id, http.StatusBadRequest, `{"code":3,"message":"invalid attachment upload"}`)
				return
			}
			request.ContentLength = int64(len(body))
			request.Header.Set("Content-Length", strconv.Itoa(len(body)))
			w := &capture{header: http.Header{}, status: http.StatusOK}
			h.ServeHTTP(w, request)
			reply(id, w.status, w.body.String())
		}()
		return nil
	})
	js.Global().Call("addEventListener", "message", cb)
	return func() {
		js.Global().Call("removeEventListener", "message", cb)
		cb.Release()
	}
}

func reply(id string, status int, body string) {
	js.Global().Call("postMessage", map[string]any{"cxz": MessageResult, "id": id, "status": status, "body": body})
}

// capture is the response, kept rather than written. The handler asks for a
// [http.ResponseController] to bound a stalled body read; there is no
// connection here to set a deadline on, and the handler already ignores a
// refusal, so none of the optional interfaces are implemented.
type capture struct {
	header  http.Header
	body    bytes.Buffer
	status  int
	written bool
}

func (c *capture) Header() http.Header { return c.header }
func (c *capture) WriteHeader(status int) {
	if !c.written {
		c.status, c.written = status, true
	}
}
func (c *capture) Write(p []byte) (int, error) {
	c.written = true
	return c.body.Write(p)
}
