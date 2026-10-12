// Package attachmenthttp carries file bodies without protobuf chunk messages.
// Its handler is private to the existing Manager socket; browser auth belongs
// to the web gateway, and local/SSH clients use their existing trusted bridge.
package attachmenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/assets"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const Prefix = "/attachments/"
const ProtocolHeader = "Cxz-Attachment-Protocol"

func Parse(r *http.Request) (assets.Upload, error) {
	q := r.URL.Query()
	size, err := strconv.ParseInt(q.Get("size"), 10, 64)
	in := assets.Upload{SessionID: strings.TrimPrefix(r.URL.Path, Prefix), RunID: q.Get("run"), Name: q.Get("name"), Size: size}
	if r.Method != http.MethodPost || !assets.ValidID(in.SessionID) || in.RunID == "" || len(in.RunID) > 128 || !assets.ValidName(in.Name) || err != nil || size < 0 || size > assets.MaxSize || (r.ContentLength >= 0 && r.ContentLength != size) {
		return in, status.Error(codes.InvalidArgument, "invalid attachment upload")
	}
	return in, nil
}

type Handler struct{ Uploader assets.Uploader }

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead && r.URL.Path == Prefix {
		w.Header().Set(ProtocolHeader, "1")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	in, err := Parse(r)
	if err != nil {
		Error(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(10 * time.Minute))
	body := http.MaxBytesReader(w, r.Body, assets.MaxSize+1)
	// Closing a stalled body also unblocks reads when logout or shutdown cancels
	// a proxied request, rather than waiting for the read deadline to expire.
	stop := context.AfterFunc(ctx, func() { _ = controller.SetReadDeadline(time.Now()); _ = body.Close() })
	defer stop()
	path, err := h.Uploader.UploadAttachment(ctx, in, body)
	if err != nil {
		Error(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Path string `json:"path"`
	}{path})
}

func Error(w http.ResponseWriter, err error) {
	code := status.Code(err)
	if errors.Is(err, assets.ErrSizeMismatch) || errors.Is(err, io.ErrUnexpectedEOF) {
		code = codes.InvalidArgument
	}
	if errors.Is(err, context.Canceled) {
		code = codes.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		code = codes.DeadlineExceeded
	}
	value := http.StatusInternalServerError
	switch code {
	case codes.InvalidArgument:
		value = http.StatusBadRequest
	case codes.NotFound:
		value = http.StatusNotFound
	case codes.FailedPrecondition:
		value = http.StatusConflict
	case codes.Unauthenticated:
		value = http.StatusUnauthorized
	case codes.PermissionDenied:
		value = http.StatusForbidden
	case codes.Unimplemented:
		value = http.StatusNotImplemented
	case codes.Unavailable:
		value = http.StatusServiceUnavailable
	case codes.DeadlineExceeded:
		value = http.StatusGatewayTimeout
	case codes.Canceled:
		value = http.StatusRequestTimeout
	}
	var large *http.MaxBytesError
	if errors.As(err, &large) {
		value = http.StatusRequestEntityTooLarge
		code = codes.ResourceExhausted
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(value)
	_ = json.NewEncoder(w).Encode(struct {
		Code    int32  `json:"code"`
		Message string `json:"message"`
	}{int32(code), status.Convert(err).Message()})
}
