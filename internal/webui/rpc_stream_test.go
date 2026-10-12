package webui

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

func TestRPCStreamAuthenticationFramesAndCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	fixture := &testSessions{auxCanceled: make(chan struct{})}
	g := grpc.NewServer()
	resource.RegisterSessionServiceServer(g, fixture)
	defer g.Stop()
	server := httptest.NewUnstartedServer(nil)
	c := Config{Listen: "127.0.0.1:0", Origin: "http://" + server.Listener.Addr().String(), Token: strings.Repeat("t", 32)}
	h, stop, err := Handler(c, nil, fstest.MapFS{"index.html": {Data: []byte("test")}}, testManagerRPC(t, g))
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	server.Config.Handler = h
	server.Start()
	defer server.Close()
	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/auth/login", strings.NewReader(`{"token":"`+c.Token+`"}`))
	req.Header.Set("Origin", c.Origin)
	login, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	cookie := login.Cookies()[0]
	headers := http.Header{"Origin": {c.Origin}, "Cookie": {cookie.Name + "=" + cookie.Value}}
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/rpc-stream/cxz.SessionService/Events"
	for _, test := range []struct {
		headers http.Header
		want    int
	}{
		{http.Header{"Origin": {c.Origin}}, 401},
		{http.Header{"Origin": {"https://foreign.test"}, "Cookie": headers.Values("Cookie")}, 403},
	} {
		_, response, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: test.headers})
		if err == nil || response.StatusCode != test.want {
			t.Fatal("unexpected authentication result", response, err)
		}
	}
	open := func(method, body string) *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/rpc-stream/cxz.SessionService/"+method, &websocket.DialOptions{HTTPHeader: headers})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.CloseNow() })
		if err := wsjson.Write(ctx, conn, map[string]any{"headers": map[string]string{"content-type": "application/connect+json", "connect-protocol-version": "1"}}); err != nil {
			t.Fatal(err)
		}
		frame := make([]byte, 5+len(body))
		binary.BigEndian.PutUint32(frame[1:5], uint32(len(body)))
		copy(frame[5:], body)
		if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
			t.Fatal(err)
		}
		var head struct {
			Status  int               `json:"status"`
			Headers map[string]string `json:"headers"`
		}
		if err := wsjson.Read(ctx, conn, &head); err != nil || head.Status != 200 || !strings.Contains(head.Headers["Content-Type"], "application/connect+json") {
			t.Fatal(head, err)
		}
		return conn
	}
	socket := open("Events", `{"afterSeq":"10"}`)
	var body []byte
	for {
		kind, data, err := socket.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if kind == websocket.MessageBinary {
			body = append(body, data...)
			continue
		}
		var end struct {
			Done bool `json:"done"`
		}
		if json.Unmarshal(data, &end) != nil || !end.Done {
			t.Fatal("invalid stream end")
		}
		break
	}
	if len(body) < 5 {
		t.Fatal("empty response")
	}
	n := int(binary.BigEndian.Uint32(body[1:5]))
	var event map[string]any
	if json.Unmarshal(body[5:5+n], &event) != nil || event["seq"] != "11" || event["text"] != "안녕" {
		t.Fatal(event)
	}
	if len(body[5+n:]) < 5 || body[5+n] != 2 {
		t.Fatal("Connect trailer was lost")
	}
	active := open("AuxEvents", `{}`)
	if _, _, err := active.Read(ctx); err != nil {
		t.Fatal(err)
	}
	logout, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/auth/logout", nil)
	logout.Header = headers.Clone()
	out, err := http.DefaultClient.Do(logout)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, out.Body)
	out.Body.Close()
	select {
	case <-fixture.auxCanceled:
	case <-ctx.Done():
		t.Fatal("logout did not cancel native RPC")
	}
}
