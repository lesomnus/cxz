package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/attachmenthttp"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// The Manager's attachment store, as far as the gateway's tests need one: the
// gateway forwards a request body, and what this records is what arrived.
type attachmentFixture struct {
	in       assets.Upload
	content  []byte
	run      string
	validate func(assets.Upload, []byte) error
}

func (f *attachmentFixture) UploadAttachment(_ context.Context, in assets.Upload, body io.Reader) (string, error) {
	run := f.run
	if run == "" {
		run = "run-current"
	}
	if in.RunID != run {
		return "", status.Error(codes.FailedPrecondition, "session run changed")
	}
	content, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	f.in, f.content = in, content
	if int64(len(content)) != in.Size {
		return "", status.Error(codes.InvalidArgument, "size mismatch")
	}
	if f.validate != nil {
		if err := f.validate(in, content); err != nil {
			return "", err
		}
	}
	return "/cxz/assets/" + in.SessionID + "/upload/" + in.Name, nil
}

func TestBrowserAttachmentStreamsWithAuthenticationAndRunBinding(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	g := grpc.NewServer()
	fixture := &attachmentFixture{}
	resource.RegisterSessionServiceServer(g, &resource.UnimplementedSessionServiceServer{})
	go g.Serve(listener)
	defer g.Stop()
	conn, err := grpc.NewClient("passthrough:///upload", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := Config{Listen: "127.0.0.1:0", Origin: "https://cxz.test", Certificate: "fixture", Key: "fixture", Token: strings.Repeat("a", 32)}
	h, closeHandler, err := Handler(c, conn, fstest.MapFS{"index.html": {Data: []byte("fixture")}}, testManagerRPC(t, g, attachmenthttp.Handler{Uploader: fixture}))
	if err != nil {
		t.Fatal(err)
	}
	defer closeHandler()
	request := func(path string, body []byte, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", c.Origin+path, bytes.NewReader(body))
		r.Header.Set("Origin", origin)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	login := request("/auth/login", []byte(`{"token":"`+c.Token+`"}`), c.Origin, nil)
	if login.Code != 204 {
		t.Fatal(login.Code, login.Body)
	}
	cookie := login.Result().Cookies()[0]
	// Larger than the JSON RPC body bound, and binary bytes remain exact. The
	// client never supplies a destination path.
	content := bytes.Repeat([]byte{0, 0xff, 0x0a, 0x80}, (9<<20)/4)
	query := url.Values{"run": {"run-current"}, "name": {"한글 report.bin"}, "size": {strconv.Itoa(len(content))}}
	path := "/attachments/session?" + query.Encode()
	if w := request(path, content, c.Origin, nil); w.Code != 401 {
		t.Fatal("unauthenticated upload", w.Code)
	}
	if w := request(path, content, "https://elsewhere.test", cookie); w.Code != 403 {
		t.Fatal("cross-origin upload", w.Code)
	}
	w := request(path, content, c.Origin, cookie)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var result struct{ Path string }
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Path != "/cxz/assets/session/upload/한글 report.bin" || fixture.in.SessionID != "session" || fixture.in.RunID != "run-current" || !bytes.Equal(fixture.content, content) {
		t.Fatal("attachment identity or bytes changed")
	}
	query.Set("run", "stale")
	if w := request("/attachments/session?"+query.Encode(), content, c.Origin, cookie); w.Code != 409 {
		t.Fatal("stale run accepted", w.Code, w.Body)
	}
	query.Set("size", strconv.FormatInt(1<<30+1, 10))
	if w := request("/attachments/session?"+query.Encode(), nil, c.Origin, cookie); w.Code != 400 {
		t.Fatal("oversize header accepted", w.Code)
	}
	query.Set("size", "1")
	if w := request("/attachments/session?"+query.Encode(), []byte("two"), c.Origin, cookie); w.Code != 400 {
		t.Fatal("body size mismatch accepted", w.Code)
	}
}
