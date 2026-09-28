package lifecycle

import (
	"bytes"
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"
)

type downloadRuntime struct {
	*countedRuntime
	data      []byte
	cancelled chan struct{}
}

func (f *downloadRuntime) Download(ctx context.Context, project, path string, w io.Writer) error {
	if project != f.p.Id {
		return status.Error(codes.NotFound, "wrong project")
	}
	if path == "denied" {
		return status.Error(codes.PermissionDenied, "file denied")
	}
	if path == "cancel" {
		w.Write([]byte("first"))
		<-ctx.Done()
		close(f.cancelled)
		return ctx.Err()
	}
	_, err := w.Write(f.data)
	return err
}

type rejectingWriter struct{}

func (rejectingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestDownloadRPCStreamsBinaryAndCancels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &downloadRuntime{countedRuntime: &countedRuntime{fixture: &fixture{p: &api.Project{Id: "project", Name: "workspace", Workspace: "/workspace", State: "running"}}}, data: bytes.Repeat([]byte{0, 255, 13, 10}, 40000), cancelled: make(chan struct{})}
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stack.Project().Add(ctx, resource.ProjectAddRequest_builder{Workspace: "/workspace"}.Build()); err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	resource.RegisterServer(server, stack)
	go server.Serve(listener)
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///download", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := resourceclient.New(conn)
	var out bytes.Buffer
	if err := client.Download(ctx, "project", "/file", &out); err != nil || !bytes.Equal(out.Bytes(), f.data) {
		t.Fatal("binary transfer failed", err)
	}
	if err := client.Download(ctx, "project", "denied", io.Discard); status.Code(err) != codes.PermissionDenied {
		t.Fatal(err)
	}
	if err := client.Download(ctx, "project", "cancel", rejectingWriter{}); err != io.ErrClosedPipe {
		t.Fatal(err)
	}
	select {
	case <-f.cancelled:
	case <-ctx.Done():
		t.Fatal("writer failure did not cancel RPC")
	}
	if err := client.Download(ctx, "project", "", io.Discard); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	if err := client.Download(ctx, "missing", "/file", io.Discard); status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
}
