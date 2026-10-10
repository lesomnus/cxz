package webui

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// A real local PTY for browser/gateway tests. Production only delegates to the
// existing Manager, which resolves the owned devcontainer and remote user.
func (*fixtureProjects) Terminal(stream grpc.BidiStreamingServer[resource.ProjectTerminalRequest, resource.ProjectTerminalReply]) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	if first.GetRef().GetRuntimeId() != "project" {
		return io.EOF
	}
	dir, err := os.MkdirTemp("", "cxz-web-terminal-fixture-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cmd := exec.CommandContext(stream.Context(), "sh", "-i")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "PS1=fixture$ ")
	pty, err := containerterm.StartPTY(cmd, int(first.GetColumns()), int(first.GetRows()))
	if err != nil {
		return err
	}
	defer pty.Close()
	stop := context.AfterFunc(stream.Context(), func() { pty.Close() })
	defer stop()
	if err = stream.Send(resource.ProjectTerminalReply_builder{Ready: boolPointer(true)}.Build()); err != nil {
		pty.Close()
		pty.Wait()
		return err
	}
	go func() {
		defer pty.Close()
		for {
			r, err := stream.Recv()
			if err != nil {
				return
			}
			if r.HasInput() {
				_, err = pty.Write(r.GetInput())
			} else {
				err = pty.Resize(int(r.GetColumns()), int(r.GetRows()))
			}
			if err != nil {
				return
			}
		}
	}()
	buf := make([]byte, 32768)
	for {
		n, err := pty.Read(buf)
		if n > 0 {
			if e := stream.Send(resource.ProjectTerminalReply_builder{Output: append([]byte(nil), buf[:n]...)}.Build()); e != nil {
				pty.Close()
				pty.Wait()
				return e
			}
		}
		if err != nil {
			break
		}
	}
	_ = pty.Wait()
	return stream.Send(resource.ProjectTerminalReply_builder{Exited: boolPointer(true)}.Build())
}

func TestTerminalGatewayPTYAuthenticationResizeFlowAndRevocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix PTY fixture")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	ln := bufconn.Listen(1 << 20)
	g := grpc.NewServer()
	resource.RegisterProjectServiceServer(g, &fixtureProjects{})
	go g.Serve(ln)
	defer g.Stop()
	conn, err := grpc.NewClient("passthrough:///terminal", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	server := httptest.NewUnstartedServer(nil)
	config := Config{Listen: "127.0.0.1:0", Origin: "http://" + server.Listener.Addr().String(), Token: strings.Repeat("t", 32)}
	h, stop, err := Handler(config, conn, fstest.MapFS{"index.html": {Data: []byte("fixture")}})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	server.Config.Handler = h
	server.Start()
	defer server.Close()
	request := func(method, path, body string, cookie *http.Cookie) *http.Response {
		r, _ := http.NewRequestWithContext(ctx, method, server.URL+path, strings.NewReader(body))
		r.Header.Set("Origin", config.Origin)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := request("GET", "/terminal/project?columns=80&rows=20", "", nil)
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("unauthenticated terminal", res.StatusCode)
	}
	res = request("POST", "/auth/login", `{"token":"`+config.Token+`"}`, nil)
	res.Body.Close()
	cookie := res.Cookies()[0]
	for _, path := range []string{"/terminal/project?columns=501&rows=20", "/terminal/project?columns=80&rows=0", "/terminal/project/path?columns=80&rows=20"} {
		res = request("GET", path, "", cookie)
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Fatal("invalid terminal dimensions/path accepted", path, res.StatusCode)
		}
	}
	dial := func(origin string) (*websocket.Conn, *http.Response, error) {
		return websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/terminal/project?columns=80&rows=20", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {origin}, "Cookie": {cookie.Name + "=" + cookie.Value}}})
	}
	_, denied, err := dial("https://foreign.test")
	if err == nil || denied.StatusCode != 403 {
		t.Fatal("foreign origin accepted", err)
	}
	_, denied, err = dial("")
	if err == nil || denied.StatusCode != 400 {
		t.Fatal("missing origin accepted", err)
	}
	c, _, err := dial(config.Origin)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	var ready terminalStatus
	if err = wsjson.Read(ctx, c, &ready); err != nil || !ready.Ready {
		t.Fatal("not ready", err, ready)
	}
	readUntil := func(want string) string {
		var out strings.Builder
		for !strings.Contains(out.String(), want) {
			kind, data, err := c.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if kind != websocket.MessageBinary {
				t.Fatalf("unexpected status: %s", data)
			}
			out.Write(data)
			if err = wsjson.Write(ctx, c, terminalControl{Ack: len(data)}); err != nil {
				t.Fatal(err)
			}
		}
		return out.String()
	}
	readUntil("fixture$ ")
	if err = wsjson.Write(ctx, c, terminalControl{Columns: 101, Rows: 23}); err != nil {
		t.Fatal(err)
	}
	if err = c.Write(ctx, websocket.MessageBinary, []byte("stty size; printf 'value:%s\\n' shell-ok\r")); err != nil {
		t.Fatal(err)
	}
	out := readUntil("value:shell-ok")
	if !strings.Contains(out, "23 101") {
		t.Fatalf("resize not applied: %q", out)
	}
	// A producer cannot bypass parse acknowledgements to grow browser buffers.
	if err = c.Write(ctx, websocket.MessageBinary, []byte("yes flow\r")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = c.Read(ctx); err != nil {
		t.Fatal(err)
	} // Deliberately don't ack.
	res = request("POST", "/auth/logout", "", cookie)
	res.Body.Close()
	// Frames already sent within the bounded window can precede the close.
	buffered := 0
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			break
		}
		buffered += len(data)
		if buffered > terminalOutputHigh {
			t.Fatal("logout did not close terminal within its output window")
		}
	}
	res = request("GET", "/terminal/project?columns=80&rows=20", "", cookie)
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("revoked terminal access")
	}
}
