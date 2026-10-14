package attachmenthttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/assets"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRawHTTPStorageValidationCancellationAndPurge(t *testing.T) {
	root := t.TempDir()
	started, finished := make(chan struct{}), make(chan error, 1)
	h := Handler{Uploader: UploaderFunc(func(ctx context.Context, in assets.Upload, src io.Reader) (string, error) {
		if in.RunID != "run" {
			return "", status.Error(codes.FailedPrecondition, "session run changed")
		}
		if in.Name == "partial.txt" {
			src = &firstRead{Reader: src, started: started}
		}
		path, err := assets.Add(ctx, root, "project", in.SessionID, in.Name, in.Size, src)
		if in.Name == "partial.txt" {
			finished <- err
		}
		return path, err
	})}
	server := httptest.NewServer(h)
	defer server.Close()
	client := &Client{Origin: server.URL, HTTP: server.Client()}
	if supported, err := client.Supported(t.Context()); err != nil || !supported {
		t.Fatal(supported, err)
	}
	data := bytes.Repeat([]byte{0, 255, 10, 128}, (9<<20)/4)
	in := assets.Upload{SessionID: "session", RunID: "run", Name: "한글 report.bin", Size: int64(len(data))}
	path, err := client.UploadAttachment(t.Context(), in, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(stored, data) || filepath.Base(path) != in.Name {
		t.Fatal("content or filename changed", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0444 {
		t.Fatal("mutable attachment", err)
	}
	for _, tc := range []struct {
		name string
		size int64
		body string
	}{{"short", 4, "abc"}, {"oversized", 2, "abc"}, {"empty", 0, ""}} {
		t.Run(tc.name, func(t *testing.T) {
			request := assets.Upload{SessionID: "session", RunID: "run", Name: tc.name + ".txt", Size: tc.size}
			_, err := client.UploadAttachment(t.Context(), request, strings.NewReader(tc.body))
			if tc.name == "empty" {
				if err != nil {
					t.Fatal(err)
				}
			} else if status.Code(err) != codes.InvalidArgument {
				t.Fatal("size validation lost", err)
			}
		})
	}
	stale := in
	stale.RunID = "stale"
	if _, err := client.UploadAttachment(t.Context(), stale, bytes.NewReader(data)); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("stale run accepted", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	pipe, writer := io.Pipe()
	request := assets.Upload{SessionID: "session", RunID: "run", Name: "partial.txt", Size: 6}
	result := make(chan error, 1)
	go func() { _, err := client.UploadAttachment(ctx, request, pipe); result <- err }()
	if _, err := writer.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("upload did not start")
	}
	cancel()
	writer.Close()
	select {
	case err := <-result:
		if status.Code(err) != codes.Canceled {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client cancellation blocked")
	}
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("partial attachment committed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("storage cancellation blocked")
	}
	entries, err := os.ReadDir(filepath.Join(assets.ExportRoot(root, "project"), "session"))
	if err != nil || len(entries) != 2 {
		t.Fatal("partial upload exposed", len(entries), err)
	}
	if err := assets.Forget(t.Context(), root, "project", "session"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("purge retained export", err)
	}
}

type firstRead struct {
	io.Reader
	started chan struct{}
}

func (r *firstRead) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if r.started != nil {
		close(r.started)
		r.started = nil
	}
	return n, err
}

func TestRawHTTPDiscoveryRedirectsAndNoPostReplay(t *testing.T) {
	var uploads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set(ProtocolHeader, "1")
			w.WriteHeader(204)
			return
		}
		uploads.Add(1)
		io.Copy(io.Discard, r.Body)
		conn, _, err := http.NewResponseController(w).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	defer server.Close()
	client := &Client{Origin: server.URL, HTTP: server.Client()}
	if supported, err := client.Supported(t.Context()); err != nil || !supported {
		t.Fatal(supported, err)
	}
	_, err := client.UploadAttachment(t.Context(), assets.Upload{SessionID: "session", RunID: "run", Name: "file.txt", Size: 4}, strings.NewReader("data"))
	if err == nil || uploads.Load() != 1 {
		t.Fatal("uncertain POST replayed", uploads.Load(), err)
	}
	for _, code := range []int{404, 501, 401, 403, 503} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
			defer s.Close()
			ok, err := (&Client{Origin: s.URL, HTTP: s.Client()}).Supported(t.Context())
			if code == 404 || code == 501 {
				if ok || err != nil {
					t.Fatal(ok, err)
				}
			} else if err == nil {
				t.Fatal("discovery failure ignored")
			}
		})
	}
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Store(true) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	_, err = (&Client{Origin: redirect.URL, HTTP: redirect.Client()}).UploadAttachment(t.Context(), assets.Upload{SessionID: "session", RunID: "run", Name: "f", Size: 4}, strings.NewReader("data"))
	if err == nil || redirected.Load() {
		t.Fatal("upload followed redirect")
	}
}

func TestRawHTTPGuardsPrecedeBodyConsumption(t *testing.T) {
	called := false
	uploader := UploaderFunc(func(context.Context, assets.Upload, io.Reader) (string, error) {
		called = true
		return "", errors.New("unexpected upload")
	})
	guard := func(_ any, _ grpc.ServerStream, info *grpc.StreamServerInfo, _ grpc.StreamHandler) error {
		if info.FullMethod != "/cxz.SessionService/Upload" {
			t.Fatal(info.FullMethod)
		}
		return status.Error(codes.Unavailable, "maintenance lease")
	}
	server := httptest.NewServer(Handler{Uploader: Guard(uploader, guard)})
	defer server.Close()
	_, err := (&Client{Origin: server.URL, HTTP: server.Client()}).UploadAttachment(t.Context(), assets.Upload{SessionID: "session", RunID: "run", Name: "file", Size: 4}, strings.NewReader("data"))
	if status.Code(err) != codes.Unavailable || called {
		t.Fatal("mutation guard bypassed", called, err)
	}
}

func TestClosingClientCancelsBlockedSourceWithoutParentCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(Handler{Uploader: UploaderFunc(func(_ context.Context, _ assets.Upload, body io.Reader) (string, error) {
		_, err := io.Copy(io.Discard, &firstRead{Reader: body, started: started})
		return "", err
	})})
	defer server.Close()
	client := &Client{Origin: server.URL, HTTP: server.Client()}
	reader, writer := io.Pipe()
	defer writer.Close()
	finished := make(chan error, 1)
	go func() {
		_, err := client.UploadAttachment(t.Context(), assets.Upload{SessionID: "session", RunID: "run", Name: "partial", Size: 8}, reader)
		finished <- err
	}()
	if _, err := writer.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("upload did not start")
	}
	client.Close()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("closed transfer accepted")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("closing client leaked an active transfer")
	}
}
