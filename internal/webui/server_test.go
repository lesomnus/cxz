package webui

import (
	"context"
	"crypto/sha256"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"connectrpc.com/connect"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

type testSessions struct {
	resource.UnimplementedSessionServiceServer
}

func TestBrowserFontPolicy(t *testing.T) {
	a := &browserAuth{origin: "https://cxz.test"}
	w := httptest.NewRecorder()
	a.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, httptest.NewRequest("GET", "https://cxz.test/", nil))
	policy := w.Header().Get("Content-Security-Policy")
	for _, directive := range []string{
		"style-src-elem 'self' 'unsafe-inline' https://fonts.googleapis.com;",
		"font-src 'self' https://fonts.gstatic.com;",
		"script-src 'self';",
		"connect-src 'self';",
	} {
		if !strings.Contains(policy, directive) {
			t.Fatalf("missing bounded font permission or altered script/RPC policy: %s", policy)
		}
	}
}

func (*testSessions) Send(_ context.Context, r *resource.SessionSendRequest) (*resource.SessionReceipt, error) {
	return resource.SessionReceipt_builder{ClientId: proto.String(r.GetClientId()), Status: proto.String(r.GetText())}.Build(), nil
}
func (*testSessions) Events(r *resource.SessionEventsRequest, s grpc.ServerStreamingServer[resource.SessionEvent]) error {
	return s.Send(resource.SessionEvent_builder{Seq: r.GetAfterSeq() + 1, Text: "안녕"}.Build())
}
func TestBrowserConnect(t *testing.T) {
	ln := bufconn.Listen(1 << 20)
	g := grpc.NewServer()
	resource.RegisterSessionServiceServer(g, &testSessions{})
	go g.Serve(ln)
	defer g.Stop()
	conn, err := grpc.NewClient("passthrough:///test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := Config{Listen: "127.0.0.1:0", Origin: "https://cxz.test", Certificate: "cert", Key: "key", Token: strings.Repeat("a", 32)}
	h, closeHandler, err := Handler(c, conn, fstest.MapFS{"index.html": {Data: []byte("cxz")}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeHandler()
	request := func(path, body, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "https://cxz.test"+path, strings.NewReader(body))
		r.Header.Set("Origin", origin)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		path, body, origin string
		code               int
	}{
		{"/auth/login", `{"token":"wrong"}`, c.Origin, 401},
		{"/auth/login", `{"token":"` + c.Token + `"}`, "https://evil.test", 403},
		{"/auth/login", `{"token":"` + c.Token + `"}`, "", 403},
		{"/cxz.SessionService/Send", `{}`, c.Origin, 401},
	} {
		if w := request(tc.path, tc.body, tc.origin, nil); w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body)
		}
	}
	w := request("/auth/login", `{"token":"`+c.Token+`"}`, c.Origin, nil)
	if w.Code != 204 {
		t.Fatal(w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe cookie")
	}
	// Exercise actual Connect framing and protobuf conversion, not a mocked RPC.
	server := httptest.NewServer(h)
	defer server.Close()
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r.Host = "cxz.test"
		r.Header.Set("Origin", c.Origin)
		r.AddCookie(cookie)
		return http.DefaultTransport.RoundTrip(r)
	})}
	send := connect.NewClient[resource.SessionSendRequest, resource.SessionReceipt](client, server.URL+"/cxz.SessionService/Send", connect.WithProtoJSON())
	reply, err := send.CallUnary(context.Background(), connect.NewRequest(resource.SessionSendRequest_builder{ClientId: proto.String("once"), Text: proto.String("hello")}.Build()))
	if err != nil {
		t.Fatal(err)
	}
	if reply.Msg.GetClientId() != "once" || reply.Msg.GetStatus() != "hello" {
		t.Fatal(reply.Msg)
	}
	events := connect.NewClient[resource.SessionEventsRequest, resource.SessionEvent](client, server.URL+"/cxz.SessionService/Events")
	stream, err := events.CallServerStream(context.Background(), connect.NewRequest(resource.SessionEventsRequest_builder{AfterSeq: proto.Uint64(41)}.Build()))
	if err != nil {
		t.Fatal(err)
	}
	if !stream.Receive() || stream.Msg().GetSeq() != 42 || stream.Msg().GetText() != "안녕" {
		t.Fatalf("stream: %v", stream.Err())
	}
	if stream.Receive() || stream.Err() != nil {
		t.Fatalf("end: %v", stream.Err())
	}
	if w := request("/auth/logout", "", c.Origin, cookie); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w := request("/cxz.SessionService/Send", "{}", c.Origin, cookie); w.Code != 401 {
		t.Fatal("logout did not revoke access")
	}
	r, _ := http.NewRequest("GET", server.URL+"/", nil)
	r.Host = "evil.test"
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	io.Copy(io.Discard, res.Body)
	if res.StatusCode != 403 {
		t.Fatal("host not checked")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSessionRevocationCancelsActiveRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	key := sha256.Sum256([]byte("cookie"))
	a := &browserAuth{sessions: map[[32]byte]browserSession{key: {ctx: ctx, cancel: cancel}}}
	s, ok := a.session("cookie")
	if !ok {
		t.Fatal("session missing")
	}
	a.mu.Lock()
	a.revoke(key)
	a.mu.Unlock()
	select {
	case <-s.ctx.Done():
	default:
		t.Fatal("active stream context was not canceled")
	}
	if _, ok := a.session("cookie"); ok {
		t.Fatal("revoked session accepted")
	}
}

func TestConfigRequiresHTTPSAndExplicitCredentials(t *testing.T) {
	c := Config{Listen: "127.0.0.1:0", Origin: "https://example.test", Certificate: "cert", Key: "key", Token: strings.Repeat("a", 32)}
	for _, origin := range []string{"http://example.test", "http://192.0.2.10:7350", "https://user@example.test", "https://example.test/path", "https://example.test?query=1", "https://example.test#fragment"} {
		bad := c
		bad.Origin = origin
		if bad.Validate() == nil {
			t.Fatalf("accepted %s", origin)
		}
	}
	if bad := (Config{Listen: "127.0.0.1:0", Origin: "https://example.test", Token: strings.Repeat("a", 32)}); bad.Validate() == nil {
		t.Fatal("accepted an https origin with no certificate")
	}
	c.Token = "short"
	if c.Validate() == nil {
		t.Fatal("accepted short token")
	}
}

// Plaintext is the desktop path: the browser already treats a loopback origin
// as secure, so no certificate exists to serve -- and one configured alongside
// it would mean the origin and the transport disagree.
func TestLoopbackOriginServesPlaintextWithoutACertificate(t *testing.T) {
	for _, origin := range []string{"http://127.0.0.1:7350", "http://localhost:7350", "http://[::1]:7350"} {
		c := Config{Listen: "127.0.0.1:7350", Origin: origin, Token: strings.Repeat("a", 32)}
		if err := c.Validate(); err != nil {
			t.Fatalf("%s: %v", origin, err)
		}
		if !c.plaintext() {
			t.Fatalf("%s is not plaintext", origin)
		}
		c.Certificate, c.Key = "cert", "key"
		if c.Validate() == nil {
			t.Fatalf("%s accepted a certificate", origin)
		}
	}
	if Loopback("example.test") || Loopback("192.0.2.10") || Loopback("") {
		t.Fatal("a routable host was read as loopback")
	}
}
