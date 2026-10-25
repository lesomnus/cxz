package webui

import (
	"context"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

type loginFixture struct {
	resource.UnimplementedProjectServiceServer
	first    chan *resource.ProjectLoginRequest
	canceled chan struct{}
}

func (p *loginFixture) SessionLogin(stream grpc.BidiStreamingServer[resource.ProjectLoginRequest, resource.ProjectLoginOutput]) error {
	return p.login(stream)
}
func (p *loginFixture) AuxiliaryLogin(stream grpc.BidiStreamingServer[resource.ProjectLoginRequest, resource.ProjectLoginOutput]) error {
	return p.login(stream)
}
func (p *loginFixture) login(stream grpc.BidiStreamingServer[resource.ProjectLoginRequest, resource.ProjectLoginOutput]) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	p.first <- first
	defer func() { p.canceled <- struct{}{} }()
	if err := stream.Send(resource.ProjectLoginOutput_builder{Output: []byte("https://provider.example/login")}.Build()); err != nil {
		return err
	}
	input, err := stream.Recv()
	if err != nil {
		return err
	}
	if string(input.GetInput()) != "code\n" {
		return io.ErrUnexpectedEOF
	}
	return stream.Send(resource.ProjectLoginOutput_builder{Output: []byte("Authenticated")}.Build())
}
func TestLoginBridgeInputCompletionAndLogoutCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	fixture := &loginFixture{first: make(chan *resource.ProjectLoginRequest, 4), canceled: make(chan struct{}, 4)}
	listener := bufconn.Listen(1 << 20)
	g := grpc.NewServer()
	resource.RegisterProjectServiceServer(g, fixture)
	go g.Serve(listener)
	defer g.Stop()
	conn, err := grpc.NewClient("passthrough:///login", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	server := httptest.NewUnstartedServer(nil)
	config := Config{Listen: "127.0.0.1:0", Origin: "http://" + server.Listener.Addr().String(), Token: strings.Repeat("t", 32)}
	handler, stop, err := Handler(config, conn, fstest.MapFS{"index.html": {Data: []byte("test")}}, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	server.Config.Handler = handler
	server.Start()
	defer server.Close()
	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/auth/login", strings.NewReader(`{"token":"`+config.Token+`"}`))
	req.Header.Set("Origin", config.Origin)
	reply, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	reply.Body.Close()
	cookie := reply.Cookies()[0]
	headers := http.Header{"Origin": {config.Origin}, "Cookie": {cookie.Name + "=" + cookie.Value}}
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/login/session?project=project&account=work&key=creation"
	_, unauthenticated, authErr := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {config.Origin}}})
	if authErr == nil || unauthenticated.StatusCode != 401 {
		t.Fatal("unauthenticated login accepted", authErr)
	}
	wrong := headers.Clone()
	wrong.Set("Origin", "https://foreign.test")
	_, denied, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: wrong})
	if err == nil || denied.StatusCode != 403 {
		t.Fatal("foreign origin accepted", err)
	}
	socket, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	defer socket.CloseNow()
	var frame loginFrame
	if err := wsjson.Read(ctx, socket, &frame); err != nil || !strings.Contains(frame.Output, "provider") {
		t.Fatal(frame, err)
	}
	first := <-fixture.first
	if projectid.RefString(first.GetRef()) != projectid.Canonical("project") || first.GetAccount().GetAlias() != "work" || first.GetSessionKey() != "creation" {
		t.Fatal(first)
	}
	if err := socket.Write(ctx, websocket.MessageText, []byte("code\n")); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Read(ctx, socket, &frame); err != nil || frame.Output != "Authenticated" {
		t.Fatal(frame, err)
	}
	frame = loginFrame{}
	if err := wsjson.Read(ctx, socket, &frame); err != nil || !frame.Done {
		t.Fatal(frame, err)
	}
	<-fixture.canceled
	socket2, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/login/auxiliary?account=work", &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	defer socket2.CloseNow()
	frame = loginFrame{}
	if err := wsjson.Read(ctx, socket2, &frame); err != nil {
		t.Fatal(err)
	}
	first = <-fixture.first
	if first.HasRef() || first.HasSessionKey() || first.GetAccount().GetAlias() != "work" {
		t.Fatal(first)
	}
	req, _ = http.NewRequestWithContext(ctx, "POST", server.URL+"/auth/logout", nil)
	req.Header.Set("Origin", config.Origin)
	req.AddCookie(cookie)
	reply, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	reply.Body.Close()
	select {
	case <-fixture.canceled:
	case <-ctx.Done():
		t.Fatal("logout left provider login running")
	}
}
