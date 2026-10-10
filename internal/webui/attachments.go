package webui

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A raw HTTP body bridges the existing client-streaming Upload RPC. Browser
// Connect transports cannot send client streams. Never buffer files on disk or
// accept a client-selected destination; the Manager owns attachment storage.
type attachmentProxy struct{ client resource.SessionServiceClient }

func (p *attachmentProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/attachments/")
	q := r.URL.Query()
	size, err := strconv.ParseInt(q.Get("size"), 10, 64)
	name, run := q.Get("name"), q.Get("run")
	if r.Method != http.MethodPost || !assets.ValidID(id) || run == "" || name == "" || name == "." || name == ".." || len(name) > 255 || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\") || strings.ContainsFunc(name, unicode.IsControl) || err != nil || size < 0 || size > assets.MaxSize || (r.ContentLength >= 0 && r.ContentLength != size) {
		http.Error(w, "invalid attachment upload", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	// Bound a stalled request body too, rather than just the upstream RPC.
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Minute))
	stream, err := p.client.Upload(ctx)
	finish := func() (string, error) {
		if stream == nil {
			return "", err
		}
		reply, e := stream.CloseAndRecv()
		if e != nil {
			return "", e
		}
		return reply.GetPath(), nil
	}
	if err == nil {
		err = stream.Send(resource.SessionUploadRequest_builder{Ref: resource.SessionRef_builder{RuntimeId: &id}.Build(), RunId: &run, Name: &name, Size: &size}.Build())
	}
	if err == nil {
		buffer := make([]byte, 256*1024)
		for {
			n, readErr := r.Body.Read(buffer)
			if n > 0 {
				// Send marshals synchronously, so the buffer can be reused.
				err = stream.Send(resource.SessionUploadRequest_builder{Content: buffer[:n]}.Build())
			}
			if err != nil || readErr == io.EOF {
				break
			}
			if readErr != nil {
				err = readErr
				break
			}
		}
	}
	var path string
	if err == nil || errors.Is(err, io.EOF) {
		path, err = finish()
	}
	if err != nil {
		code := http.StatusBadGateway
		switch status.Code(err) {
		case codes.InvalidArgument:
			code = http.StatusBadRequest
		case codes.NotFound:
			code = http.StatusNotFound
		case codes.FailedPrecondition:
			code = http.StatusConflict
		case codes.PermissionDenied:
			code = http.StatusForbidden
		case codes.Unimplemented:
			code = http.StatusNotImplemented
		}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			code = http.StatusRequestEntityTooLarge
		}
		http.Error(w, status.Convert(err).Message(), code)
		return
	}
	if path == "" {
		http.Error(w, "upload completed without an attachment path", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Path string `json:"path"`
	}{path})
}
