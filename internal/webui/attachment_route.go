package webui

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/lesomnus/cxz/internal/attachmenthttp"
)

// Prefer a raw proxy to the storage owner. Probe without reading the upload;
// the legacy RPC adapter is used only when the Manager lacks this endpoint.
// Once POST starts, errors are returned without replaying any bytes.
type attachmentRoute struct {
	raw, legacy http.Handler
	supported   atomic.Bool
}

func (p *attachmentRoute) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	r = r.WithContext(ctx)
	if !p.supported.Load() {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		probe := r.Clone(ctx)
		probe.Method = http.MethodHead
		probe.URL.Path = attachmenthttp.Prefix
		probe.URL.RawQuery = ""
		probe.RequestURI = attachmenthttp.Prefix
		probe.ContentLength = 0
		probe.Body = http.NoBody
		probe.GetBody = nil
		probe.TransferEncoding = nil
		probe.Header.Del("Content-Length")
		probe.Header.Del("Expect")
		result := &attachmentProbe{header: make(http.Header)}
		p.raw.ServeHTTP(result, probe)
		cancel()
		if result.code == http.StatusNoContent && result.header.Get(attachmenthttp.ProtocolHeader) == "1" {
			p.supported.Store(true)
		} else if result.code == http.StatusNotFound || result.code == http.StatusNotImplemented {
			p.legacy.ServeHTTP(w, r)
			return
		} else {
			http.Error(w, "attachment endpoint discovery failed", http.StatusBadGateway)
			return
		}
	}
	p.raw.ServeHTTP(w, r)
}

type attachmentProbe struct {
	header http.Header
	code   int
}

func (p *attachmentProbe) Header() http.Header { return p.header }
func (p *attachmentProbe) WriteHeader(code int) {
	if p.code == 0 {
		p.code = code
	}
}
func (p *attachmentProbe) Write(b []byte) (int, error) {
	p.WriteHeader(http.StatusOK)
	return len(b), nil
}
