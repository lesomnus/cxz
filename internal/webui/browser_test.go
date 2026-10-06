package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/editor"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

// Opt-in fixture for Playwright: no Docker, credentials or paid agents.
func TestBrowserFixture(t *testing.T) {
	if os.Getenv("CXZ_WEB_FIXTURE") != "1" {
		t.Skip("Playwright fixture")
	}
	ln := bufconn.Listen(1 << 20)
	g := grpc.NewServer()
	f := &browserFixture{}
	resource.RegisterProjectServiceServer(g, &fixtureProjects{probe: os.Getenv("CXZ_EDITOR_PROBE_CONTAINER")})
	resource.RegisterSessionServiceServer(g, f)
	go g.Serve(ln)
	defer g.Stop()
	conn, err := grpc.NewClient("passthrough:///fixture", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// The plaintext variant is the desktop path, and what it has to prove is a
	// browser question: that Chrome gives http://127.0.0.1 a secure context and
	// so accepts the __Host- session cookie there. Only a browser can answer it.
	c := Config{Listen: "127.0.0.1:18081", Origin: "https://127.0.0.1:18081", Certificate: "fixture", Key: "fixture", Token: strings.Repeat("a", 32)}
	if os.Getenv("CXZ_WEB_FIXTURE_PLAINTEXT") == "1" {
		c = Config{Listen: "127.0.0.1:18082", Origin: "http://127.0.0.1:18082", Token: strings.Repeat("a", 32)}
	}
	h, stop, err := Handler(c, conn, Assets())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	s := httptest.NewUnstartedServer(h)
	s.Listener.Close()
	s.Listener, err = net.Listen("tcp", c.Listen)
	if err != nil {
		t.Fatal(err)
	}
	if c.plaintext() {
		s.Start()
	} else {
		s.StartTLS()
	}
	defer s.Close()
	t.Log("web fixture ready")
	time.Sleep(10 * time.Minute)
}

var fixtureProject = resource.Project_builder{Id: []byte("1234567890123456"), RuntimeId: "project", Alias: "demo", Name: "Demo project", Listed: true, Status: resource.ProjectStatus_builder{State: "running", RemoteWorkspace: "/workspace"}.Build()}.Build()

type fixtureProjects struct {
	resource.UnimplementedProjectServiceServer
	probe string
}

func (*fixtureProjects) Paths(_ *resource.ProjectPathsRequest, s grpc.ServerStreamingServer[resource.ProjectPathsReply]) error {
	name := "README.md"
	return s.Send(resource.ProjectPathsReply_builder{Entries: []*resource.ProjectPathEntry{resource.ProjectPathEntry_builder{Name: &name}.Build()}}.Build())
}
func (*fixtureProjects) Download(_ *resource.ProjectDownloadRequest, s grpc.ServerStreamingServer[resource.ProjectDownloadReply]) error {
	data := []byte("# Fixture workspace\n\nA readonly file preview.\n")
	size := int64(len(data))
	return s.Send(resource.ProjectDownloadReply_builder{Data: data, TotalSize: &size}.Build())
}

// Opt-in real IDE probe: this test-owned container already has the helper and
// pinned release installed. Ordinary browser tests still need no Docker.
func (p *fixtureProjects) Editor(ctx context.Context, _ *resource.ProjectEditorRequest) (*resource.ProjectEditorReply, error) {
	if p.probe == "" {
		return nil, fmt.Errorf("real editor probe is opt-in")
	}
	data, err := dockerx.Run(ctx, "exec", p.probe, "/tmp/cxz", "_editor-start", "project", "/workspace")
	if err != nil {
		return nil, err
	}
	var result editor.Result
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return resource.ProjectEditorReply_builder{Workspace: &result.Workspace, ConnectionToken: &result.Token}.Build(), nil
}
func (p *fixtureProjects) EditorTunnel(stream grpc.BidiStreamingServer[resource.ProjectEditorTunnelRequest, resource.ProjectEditorTunnelReply]) error {
	if p.probe == "" {
		return fmt.Errorf("real editor probe is opt-in")
	}
	if _, err := stream.Recv(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", p.probe, "/tmp/cxz", "_editor-tunnel")
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	defer out.Close()
	if err = cmd.Start(); err != nil {
		return err
	}
	defer func() { cancel(); cmd.Wait() }()
	if err = stream.Send(resource.ProjectEditorTunnelReply_builder{Ready: boolPointer(true)}.Build()); err != nil {
		return err
	}
	go func() {
		for {
			r, err := stream.Recv()
			if err != nil {
				return
			}
			if _, err = in.Write(r.GetInput()); err != nil {
				return
			}
		}
	}()
	buf := make([]byte, 65536)
	for {
		n, err := out.Read(buf)
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

func (*fixtureProjects) List(context.Context, *resource.ProjectListRequest) (*resource.ProjectListResponse, error) {
	return resource.ProjectListResponse_builder{Items: []*resource.Project{fixtureProject}}.Build(), nil
}
func (*fixtureProjects) Get(context.Context, *resource.ProjectGetRequest) (*resource.Project, error) {
	return fixtureProject, nil
}
func (*fixtureProjects) Watch(_ *resource.ProjectWatchRequest, s grpc.ServerStreamingServer[resource.ProjectWatchResponse]) error {
	<-s.Context().Done()
	return s.Context().Err()
}

type browserFixture struct {
	resource.UnimplementedSessionServiceServer
	mu       sync.Mutex
	events   []*resource.SessionEvent
	resolved bool
}

func (f *browserFixture) snapshot() *resource.Session {
	pending := []*resource.SessionEvent{}
	if !f.resolved {
		pending = append(pending, resource.SessionEvent_builder{Seq: 3, RunId: "run", Kind: "approval", Text: "AskUserQuestion", RequestId: "q", Payload: []byte(`{"input":{"questions":[{"question":"Which environment?","options":[{"label":"Development","description":"Use a disposable environment"},{"label":"Production"}]}]}}`)}.Build())
	}
	return resource.Session_builder{Id: []byte("abcdefghijklmnop"), RuntimeId: "session", Alias: "demo-chat", Name: "Demo conversation", Agent: "claude", Listed: true, Project: fixtureProject, Status: resource.SessionStatus_builder{RunId: "run", State: "idle", LastSeq: 3, Pending: pending}.Build()}.Build()
}
func (f *browserFixture) Get(context.Context, *resource.SessionGetRequest) (*resource.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshot(), nil
}
func (f *browserFixture) List(context.Context, *resource.SessionListRequest) (*resource.SessionListResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return resource.SessionListResponse_builder{Items: []*resource.Session{f.snapshot()}}.Build(), nil
}
func (f *browserFixture) Watch(_ *resource.SessionWatchRequest, s grpc.ServerStreamingServer[resource.SessionWatchResponse]) error {
	<-s.Context().Done()
	return s.Context().Err()
}
func (f *browserFixture) History(_ context.Context, r *resource.SessionEventsRequest) (*resource.SessionEventBatch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.events == nil {
		f.events = []*resource.SessionEvent{resource.SessionEvent_builder{Seq: 1, RunId: "run", Kind: "input", Text: "Show the project"}.Build(), resource.SessionEvent_builder{Seq: 2, RunId: "run", Kind: "assistant", Text: "## Ready\n\nHello **mobile**. <span style=\"position:fixed;inset:0;color:red\">style fixture</span>\n\n```go\nfmt.Println(\"안녕\")\n```\n\n<script>window.pwned=true</script>", Response: resource.ResponseMetadata_builder{Model: "fixture-model", Effort: "high", ModelSource: "response", EffortSource: "settings"}.Build()}.Build()}
		f.events[1].SetTimeMs(time.Now().UnixMilli())
		f.events = append(f.events, resource.SessionEvent_builder{Seq: 4, RunId: "run", Kind: "turn_end", Text: "completed", Response: resource.ResponseMetadata_builder{CompletionJson: []byte(`{"response_seq":"2","duration_ms":4200,"duration_source":"provider","token_scope":"turn","metrics":{"input_tokens":1200,"output_tokens":320}}`)}.Build()}.Build())
	}
	var out []*resource.SessionEvent
	for _, e := range f.events {
		if e.GetSeq() > r.GetAfterSeq() {
			out = append(out, e)
		}
	}
	return resource.SessionEventBatch_builder{Events: out}.Build(), nil
}
func (f *browserFixture) Events(r *resource.SessionEventsRequest, s grpc.ServerStreamingServer[resource.SessionEvent]) error {
	cursor := r.GetAfterSeq()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.Context().Done():
			return s.Context().Err()
		case <-ticker.C:
			f.mu.Lock()
			pending := append([]*resource.SessionEvent(nil), f.events...)
			f.mu.Unlock()
			for _, e := range pending {
				if e.GetSeq() > cursor {
					if err := s.Send(e); err != nil {
						return err
					}
					cursor = e.GetSeq()
				}
			}
		}
	}
}
func (f *browserFixture) Send(_ context.Context, r *resource.SessionSendRequest) (*resource.SessionReceipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, resource.SessionEvent_builder{Seq: uint64(10 + len(f.events)), RunId: r.GetRunId(), Kind: "input", Text: r.GetText()}.Build())
	return resource.SessionReceipt_builder{ClientId: proto.String(r.GetClientId()), Status: proto.String("accepted")}.Build(), nil
}
func (f *browserFixture) Reply(_ context.Context, r *resource.SessionReplyRequest) (*resource.SessionReceipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolved = true
	f.events = append(f.events, resource.SessionEvent_builder{Seq: uint64(10 + len(f.events)), RunId: r.GetRunId(), Kind: "approval_resolved", RequestId: r.GetRequestId(), Text: "allowed"}.Build())
	return resource.SessionReceipt_builder{ClientId: proto.String(r.GetClientId()), Status: proto.String("accepted")}.Build(), nil
}
