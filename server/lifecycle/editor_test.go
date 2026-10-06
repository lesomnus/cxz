package lifecycle

import (
	"bytes"
	"context"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/editor"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type editorRuntimeFixture struct {
	*countedRuntime
	closed chan struct{}
}

func (f *editorRuntimeFixture) Editor(context.Context, string) (editor.Result, error) {
	return editor.Result{Workspace: "/workspace", Token: strings.Repeat("a", 64)}, nil
}
func (f *editorRuntimeFixture) OpenEditorTunnel(ctx context.Context, id string) (io.ReadWriteCloser, error) {
	if id != f.p.Id {
		return nil, status.Error(codes.NotFound, "wrong project")
	}
	client, server := net.Pipe()
	context.AfterFunc(ctx, func() { server.Close(); f.closed <- struct{}{} })
	go func() { io.Copy(server, server); server.Close() }()
	return client, nil
}

func TestEditorLifecycleRPCRejectsDeletedProjectsAndInvalidFrames(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fixture := &editorRuntimeFixture{countedRuntime: &countedRuntime{fixture: &fixture{p: &api.Project{Id: "project", Name: "workspace", Workspace: "/workspace", State: "running"}}}, closed: make(chan struct{}, 4)}
	stack, err := Build(ctx, db, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stack.Project().Add(ctx, resource.ProjectAddRequest_builder{Workspace: "/workspace"}.Build()); err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	resource.RegisterServer(server, stack)
	go server.Serve(listener)
	defer server.Stop()
	connection, err := grpc.NewClient("passthrough:///editor", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	client := resource.NewProjectServiceClient(connection)
	request := resource.ProjectEditorRequest_builder{Ref: projectRef("project")}.Build()
	reply, err := client.Editor(ctx, request)
	if err != nil || reply.GetWorkspace() != "/workspace" || reply.GetConnectionToken() != strings.Repeat("a", 64) || reply.GetSimulated() {
		t.Fatal("native startup metadata lost", err)
	}
	open := func(ctx context.Context) grpc.BidiStreamingClient[resource.ProjectEditorTunnelRequest, resource.ProjectEditorTunnelReply] {
		t.Helper()
		stream, err := client.EditorTunnel(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = stream.Send(resource.ProjectEditorTunnelRequest_builder{Ref: projectRef("project")}.Build()); err != nil {
			t.Fatal(err)
		}
		ready, err := stream.Recv()
		if err != nil || !ready.GetReady() {
			t.Fatal("tunnel startup", err)
		}
		return stream
	}
	requestCtx, abort := context.WithCancel(ctx)
	stream := open(requestCtx)
	data := []byte{0, 255, 1, 13, 10, 42}
	if err = stream.Send(resource.ProjectEditorTunnelRequest_builder{Input: data}.Build()); err != nil {
		t.Fatal(err)
	}
	echoed, err := stream.Recv()
	if err != nil || !bytes.Equal(echoed.GetOutput(), data) {
		t.Fatal("binary transport altered bytes", err)
	}
	abort()
	select {
	case <-fixture.closed:
	case <-ctx.Done():
		t.Fatal("cancel did not close runtime tunnel")
	}
	bad, err := client.EditorTunnel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = bad.Send(resource.ProjectEditorTunnelRequest_builder{Input: []byte("no ref")}.Build()); err != nil {
		t.Fatal(err)
	}
	if _, err = bad.Recv(); status.Code(err) != codes.InvalidArgument {
		t.Fatal("malformed first frame accepted", err)
	}
	for _, frame := range []*resource.ProjectEditorTunnelRequest{
		resource.ProjectEditorTunnelRequest_builder{Ref: projectRef("project")}.Build(),
		resource.ProjectEditorTunnelRequest_builder{}.Build(),
		resource.ProjectEditorTunnelRequest_builder{Input: make([]byte, 65537)}.Build(),
	} {
		stream := open(ctx)
		if err = stream.Send(frame); err != nil {
			t.Fatal(err)
		}
		if _, err = stream.Recv(); status.Code(err) != codes.InvalidArgument {
			t.Fatal("invalid continuation accepted", err)
		}
		select {
		case <-fixture.closed:
		case <-ctx.Done():
			t.Fatal("invalid stream leaked runtime tunnel")
		}
	}
	layer, _ := resource.Find[Layer](stack)
	if _, err = layer.Next().Project().Patch(ctx, resource.ProjectPatchRequest_builder{Ref: projectRef("project"), Listed: ptr(false), DateUpdatedForce: ptr(true)}.Build()); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Editor(ctx, request); status.Code(err) != codes.NotFound {
		t.Fatal("deleted project editor accepted", err)
	}
	deleted, err := client.EditorTunnel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = deleted.Send(resource.ProjectEditorTunnelRequest_builder{Ref: projectRef("project")}.Build()); err != nil {
		t.Fatal(err)
	}
	if _, err = deleted.Recv(); status.Code(err) != codes.NotFound {
		t.Fatal("deleted project tunnel accepted", err)
	}
	if fixture.snapshots.Load() != 0 {
		t.Fatal("editor effects refreshed unrelated project inventory")
	}
}
