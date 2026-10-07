package webui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"connectrpc.com/connect"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type testEditorProjects struct {
	resource.UnimplementedProjectServiceServer
	address string
	token   string
}

func (p *testEditorProjects) Get(context.Context, *resource.ProjectGetRequest) (*resource.Project, error) {
	return resource.Project_builder{RuntimeId: "project", Listed: true, Status: resource.ProjectStatus_builder{RemoteWorkspace: "/workspace"}.Build()}.Build(), nil
}
func (p *testEditorProjects) Editor(context.Context, *resource.ProjectEditorRequest) (*resource.ProjectEditorReply, error) {
	workspace := "/workspace"
	return resource.ProjectEditorReply_builder{Workspace: &workspace, ConnectionToken: &p.token}.Build(), nil
}
func (p *testEditorProjects) EditorTunnel(stream grpc.BidiStreamingServer[resource.ProjectEditorTunnelRequest, resource.ProjectEditorTunnelReply]) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	c, err := net.Dial("tcp", p.address)
	if err != nil {
		return err
	}
	defer c.Close()
	stop := context.AfterFunc(stream.Context(), func() { c.Close() })
	defer stop()
	if err = stream.Send(resource.ProjectEditorTunnelReply_builder{Ready: boolPointer(true)}.Build()); err != nil {
		return err
	}
	go func() {
		for {
			r, err := stream.Recv()
			if err != nil {
				return
			}
			if _, err = c.Write(r.GetInput()); err != nil {
				return
			}
		}
	}()
	buf := make([]byte, 65536)
	for {
		n, err := c.Read(buf)
		if n > 0 {
			if e := stream.Send(resource.ProjectEditorTunnelReply_builder{Output: append([]byte(nil), buf[:n]...)}.Build()); e != nil {
				return e
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
func (p *testEditorProjects) Download(_ *resource.ProjectDownloadRequest, stream grpc.ServerStreamingServer[resource.ProjectDownloadReply]) error {
	total := int64(2 << 20)
	return stream.Send(resource.ProjectDownloadReply_builder{TotalSize: &total}.Build())
}

func TestEditorProxyAuthenticationBytesUpgradeAndRevocation(t *testing.T) {
	token := strings.Repeat("b", 64)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("vscode-tkn")
		if err != nil || cookie.Value != token || r.URL.Query().Get("tkn") != "" || strings.Contains(r.Header.Get("Cookie"), cookieName) {
			http.Error(w, "bad upstream authentication", 403)
			return
		}
		if r.URL.Path == "/editor/project/socket" {
			c, b, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer c.Close()
			fmt.Fprint(b, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
			b.Flush()
			io.Copy(c, b)
			return
		}
		fmt.Fprint(w, r.URL.Path+" "+r.URL.Query().Get("folder"))
	}))
	defer upstream.Close()
	listener := bufconn.Listen(1 << 20)
	g := grpc.NewServer()
	resource.RegisterProjectServiceServer(g, &testEditorProjects{address: strings.TrimPrefix(upstream.URL, "http://"), token: token})
	go g.Serve(listener)
	defer g.Stop()
	conn, err := grpc.NewClient("passthrough:///editor-test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	config := Config{Listen: "127.0.0.1:0", Origin: "https://cxz.test", Certificate: "fixture", Key: "fixture", Token: strings.Repeat("a", 32)}
	handler, stop, err := Handler(config, conn, fstest.MapFS{"index.html": {Data: []byte("cxz")}})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	server := httptest.NewServer(handler)
	defer server.Close()
	request := func(method, path, body string, cookie *http.Cookie) *http.Response {
		r, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		r.Host = "cxz.test"
		r.Header.Set("Origin", config.Origin)
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	unauth := request("GET", "/editor/project/", "", nil)
	unauth.Body.Close()
	if unauth.StatusCode != 401 {
		t.Fatal(unauth.StatusCode)
	}
	login := request("POST", "/auth/login", `{"token":"`+config.Token+`"}`, nil)
	login.Body.Close()
	cookie := login.Cookies()[0]
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r.Host = "cxz.test"
		r.Header.Set("Origin", config.Origin)
		r.AddCookie(cookie)
		return http.DefaultTransport.RoundTrip(r)
	})}
	editor := connect.NewClient[resource.ProjectEditorRequest, resource.ProjectEditorReply](client, server.URL+"/cxz.ProjectService/Editor", connect.WithProtoJSON())
	id := "project"
	reply, err := editor.CallUnary(context.Background(), connect.NewRequest(resource.ProjectEditorRequest_builder{Ref: resource.ProjectRef_builder{RuntimeId: &id}.Build()}.Build()))
	if err != nil {
		t.Fatal(err)
	}
	if reply.Msg.GetConnectionToken() != "" || reply.Msg.GetWorkspace() != "/workspace" {
		t.Fatal("private token leaked")
	}
	res := request("GET", "/editor/project/?folder=%2Fworkspace", "", cookie)
	data, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || res.StatusCode != 200 || string(data) != "/editor/project/ /workspace" {
		t.Fatalf("proxy: %d %q %v", res.StatusCode, data, err)
	}
	editorCookies := res.Cookies()
	if len(editorCookies) != 1 || editorCookies[0].Name != "vscode-tkn" || editorCookies[0].Path != "/editor/project/" || !editorCookies[0].Secure || editorCookies[0].HttpOnly || editorCookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("workbench handshake cookie is not path-scoped/secure")
	}
	if res.Header.Get("X-Frame-Options") != "SAMEORIGIN" || !strings.Contains(res.Header.Get("Content-Security-Policy"), "frame-ancestors 'self'") {
		t.Fatal("iframe policy")
	}
	download := connect.NewClient[resource.ProjectDownloadRequest, resource.ProjectDownloadReply](client, server.URL+"/cxz.ProjectService/Download")
	file := "/workspace/large"
	stream, err := download.CallServerStream(context.Background(), connect.NewRequest(resource.ProjectDownloadRequest_builder{Ref: resource.ProjectRef_builder{RuntimeId: &id}.Build(), Path: &file}.Build()))
	if err != nil {
		t.Fatal(err)
	}
	if stream.Receive() || connect.CodeOf(stream.Err()) != connect.CodeResourceExhausted {
		t.Fatal("unbounded preview", stream.Err())
	}
	file = "/workspace/../secret"
	stream, err = download.CallServerStream(context.Background(), connect.NewRequest(resource.ProjectDownloadRequest_builder{Ref: resource.ProjectRef_builder{RuntimeId: &id}.Build(), Path: &file}.Build()))
	if err != nil {
		t.Fatal(err)
	}
	if stream.Receive() || connect.CodeOf(stream.Err()) != connect.CodePermissionDenied {
		t.Fatal("workspace path escaped", stream.Err())
	}
	socket, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	socket.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(socket, "GET /editor/project/socket HTTP/1.1\r\nHost: cxz.test\r\nOrigin: https://cxz.test\r\nCookie: %s=%s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n", cookie.Name, cookie.Value)
	reader := bufio.NewReader(socket)
	response, err := http.ReadResponse(reader, nil)
	if err != nil || response.StatusCode != 101 {
		t.Fatal("upgrade", err)
	}
	socket.Write([]byte("hello"))
	echo := make([]byte, 5)
	if _, err = io.ReadFull(reader, echo); err != nil || string(echo) != "hello" {
		t.Fatal("bidirectional bytes", err)
	}
	logout := request("POST", "/auth/logout", "", cookie)
	logout.Body.Close()
	socket.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err = reader.ReadByte(); err == nil {
		t.Fatal("logout did not close upgraded connection")
	}
	if n, ok := err.(net.Error); ok && n.Timeout() {
		t.Fatal("upgrade remained alive after logout")
	}
	revoked := request("GET", "/editor/project/", "", cookie)
	revoked.Body.Close()
	if revoked.StatusCode != 401 {
		t.Fatal("revoked editor access")
	}
}
