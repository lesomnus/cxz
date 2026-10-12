package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/attachmenthttp"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBrowserRawAttachmentUsesManagerBodyNotRPC(t *testing.T) {
	root := t.TempDir()
	native := &attachmentFixture{}
	g := grpc.NewServer()
	defer g.Stop()
	resource.RegisterSessionServiceServer(g, native)
	raw := attachmenthttp.Handler{Uploader: attachmenthttp.UploaderFunc(func(ctx context.Context, in assets.Upload, body io.Reader) (string, error) {
		if in.RunID != "run" {
			return "", status.Error(codes.FailedPrecondition, "session run changed")
		}
		if in.SessionID != "session" {
			return "", status.Error(codes.NotFound, "wrong session")
		}
		return assets.Add(ctx, root, "project", in.SessionID, in.Name, in.Size, body)
	})}
	c := Config{Listen: "127.0.0.1:0", Origin: "https://cxz.test", Certificate: "cert", Key: "key", Token: strings.Repeat("a", 32)}
	h, close, err := Handler(c, nil, fstest.MapFS{"index.html": {Data: []byte("test")}}, testManagerRPC(t, g, raw))
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	login := httptest.NewRequest("POST", c.Origin+"/auth/login", strings.NewReader(`{"token":"`+c.Token+`"}`))
	login.Header.Set("Origin", c.Origin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, login)
	cookie := w.Result().Cookies()[0]
	data := bytes.Repeat([]byte{0, 255, 13, 128}, (9<<20)/4)
	path := "/attachments/session?run=run&name=file.bin&size=9437184"
	request := func(path, origin string, authenticated bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", c.Origin+path, bytes.NewReader(data))
		r.Header.Set("Origin", origin)
		if authenticated {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request(path, c.Origin, false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request(path, "https://foreign.test", true); w.Code != 403 {
		t.Fatal(w.Code)
	}
	w = request(path, c.Origin, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var reply struct{ Path string }
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(reply.Path)
	if err != nil || !bytes.Equal(stored, data) {
		t.Fatal("raw body changed", err)
	}
	if native.header != nil {
		t.Fatal("file bytes were wrapped in Upload RPC")
	}
	if w := request(strings.Replace(path, "run=run", "run=stale", 1), c.Origin, true); w.Code != 409 {
		t.Fatal("run binding changed", w.Code, w.Body)
	}
}

func TestBrowserLogoutCancelsRawUploadBeforePublication(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	started := make(chan struct{})
	finished := make(chan error, 1)
	g := grpc.NewServer()
	resource.RegisterSessionServiceServer(g, &attachmentFixture{})
	defer g.Stop()
	raw := attachmenthttp.Handler{Uploader: attachmenthttp.UploaderFunc(func(ctx context.Context, in assets.Upload, body io.Reader) (string, error) {
		path, err := assets.Add(ctx, root, "project", in.SessionID, in.Name, in.Size, &uploadStartedReader{Reader: body, started: started})
		finished <- err
		return path, err
	})}
	server := httptest.NewUnstartedServer(nil)
	c := Config{Listen: "127.0.0.1:0", Origin: "http://" + server.Listener.Addr().String(), Token: strings.Repeat("a", 32)}
	h, stop, err := Handler(c, nil, fstest.MapFS{"index.html": {Data: []byte("test")}}, testManagerRPC(t, g, raw))
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	server.Config.Handler = h
	server.Start()
	defer server.Close()
	login, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/auth/login", strings.NewReader(`{"token":"`+c.Token+`"}`))
	login.Header.Set("Origin", c.Origin)
	reply, err := server.Client().Do(login)
	if err != nil {
		t.Fatal(err)
	}
	reply.Body.Close()
	cookie := reply.Cookies()[0]
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	request, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/attachments/session?run=run&name=partial.txt&size=6", reader)
	request.Header.Set("Origin", c.Origin)
	request.AddCookie(cookie)
	done := make(chan struct{})
	go func() {
		defer close(done)
		result, err := server.Client().Do(request)
		if err == nil {
			io.Copy(io.Discard, result.Body)
			result.Body.Close()
		}
	}()
	if _, err := writer.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("upload did not start")
	}
	logout, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/auth/logout", nil)
	logout.Header.Set("Origin", c.Origin)
	logout.AddCookie(cookie)
	out, err := server.Client().Do(logout)
	if err != nil {
		t.Fatal(err)
	}
	out.Body.Close()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("partial file published after logout")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("logout did not cancel raw storage")
	}
	writer.Close()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("browser upload stayed blocked")
	}
	if _, err := os.Stat(assets.ExportRoot(root, "project")); !os.IsNotExist(err) {
		t.Fatal("cancelled upload has an export", err)
	}
}

type uploadStartedReader struct {
	io.Reader
	started chan struct{}
	once    sync.Once
}

func (r *uploadStartedReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.once.Do(func() { close(r.started) })
	}
	return n, err
}
