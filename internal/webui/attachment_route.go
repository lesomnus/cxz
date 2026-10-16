package webui

import (
	"context"
	"net/http"
	"time"

	"github.com/lesomnus/cxz/internal/attachmenthttp"
)

// The browser's file body goes to the storage owner as itself.
//
// This used to probe for the endpoint and fall back to an RPC adapter when the
// Manager lacked it. There is no adapter to fall back to now, so there is
// nothing for a probe to decide: the request goes, and a Manager that does not
// serve the route says so before the body is read -- the upload carries
// `Expect: 100-continue`. Once POST starts, errors are returned without
// replaying any bytes.
type attachmentRoute struct{ raw http.Handler }

func (p *attachmentRoute) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Refused here as well as by the Manager, so a malformed browser request
	// does not cross the gateway to be refused on the other side.
	if _, err := attachmenthttp.Parse(r); err != nil {
		attachmenthttp.Error(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(10 * time.Minute))
	stop := context.AfterFunc(ctx, func() { _ = controller.SetReadDeadline(time.Now()) })
	defer stop()
	p.raw.ServeHTTP(w, r.WithContext(ctx))
}
