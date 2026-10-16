package websandbox

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/attachmenthttp"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestUploadsPreserveBytesAndEnforceRunSizeAndProjectScope(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	x := s.Sessions()
	content := []byte("한글\r\n\x00\xff")
	in := func(run string, size int64) assets.Upload {
		return assets.Upload{SessionID: "session-1", RunID: run, Name: "report.txt", Size: size}
	}
	path, err := x.UploadAttachment(t.Context(), in("session-1-run-1", int64(len(content))), bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(s.uploads[path].content, content) || s.uploadBytes != int64(len(content)) {
		t.Fatal("upload changed bytes")
	}
	p := &Projects{S: s}
	download := &fixtureStream[resource.ProjectDownloadReply]{ctx: t.Context()}
	request := resource.ProjectDownloadRequest_builder{Ref: resource.ProjectRef_builder{RuntimeId: proto.String("project-1")}.Build(), Path: &path}.Build()
	if err := p.Download(request, download); err != nil || len(download.replies) != 1 || !bytes.Equal(download.replies[0].GetData(), content) {
		t.Fatal("uploaded file not readable", err)
	}
	request.SetRef(resource.ProjectRef_builder{RuntimeId: proto.String("project-2")}.Build())
	if err := p.Download(request, download); status.Code(err) != codes.NotFound {
		t.Fatal("file crossed project boundary", err)
	}
	// A stale run, a body shorter than its declared size, and one byte longer
	// than it. The last two are the sandbox's own check: a declared prefix of a
	// longer source would publish a file nobody chose.
	for _, bad := range []struct {
		in   assets.Upload
		body string
	}{
		{in("stale", int64(len(content))), string(content)},
		{in("session-1-run-1", int64(len(content)+1)), string(content)},
		{in("session-1-run-1", int64(len(content))), string(content) + "x"},
	} {
		if _, err := x.UploadAttachment(t.Context(), bad.in, strings.NewReader(bad.body)); err == nil {
			t.Fatal("invalid upload committed")
		}
		if s.uploadBytes != int64(len(content)) || len(s.uploads) != 1 {
			t.Fatal("invalid upload changed the store")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := x.UploadAttachment(ctx, in("session-1-run-1", 0), strings.NewReader("")); err == nil {
		t.Fatal("cancelled upload committed")
	}
	if _, err := x.Purge(t.Context(), resource.SessionPurgeRequest_builder{Ref: resource.SessionRef_builder{RuntimeId: proto.String("session-1")}.Build()}.Build()); err != nil || s.uploadBytes != 0 || len(s.uploads) != 0 {
		t.Fatal("purge retained uploads", err)
	}
}

// The page uploads through the Manager's own handler, so what a sandbox refuses
// is what the real server refuses, with the same status and the same JSON. This
// is the whole reason the sandbox does not have an upload path of its own.
func TestSandboxUploadsAnswerThroughTheSharedHandler(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	h := attachmenthttp.Handler{Uploader: s.Sessions()}
	post := func(query string, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, attachmenthttp.Prefix+"session-1?"+query, strings.NewReader(body))
		r.ContentLength = int64(len(body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	content := "한글 report"
	ok := post("run=session-1-run-1&name=report.txt&size="+strconv.Itoa(len(content)), content)
	if ok.Code != http.StatusOK || !strings.Contains(ok.Body.String(), `"path":"/cxz/assets/session-1/upload-1/report.txt"`) {
		t.Fatal("upload not published", ok.Code, ok.Body.String())
	}
	// A name the shared validator refuses, and a run the sandbox refuses. Both
	// arrive as the handler's own status rather than as something a page would
	// have to learn only here.
	for query, want := range map[string]int{
		"run=session-1-run-1&name=../escape&size=1": http.StatusBadRequest,
		"run=session-1-run-1&name=&size=1":          http.StatusBadRequest,
		"run=stale&name=report.txt&size=1":          http.StatusConflict,
	} {
		if got := post(query, "x"); got.Code != want {
			t.Fatal("wrong status for", query, got.Code, got.Body.String())
		}
	}
	if s.uploadBytes != int64(len(content)) || len(s.uploads) != 1 {
		t.Fatal("a refused upload reached the store")
	}
}
