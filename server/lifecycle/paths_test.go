package lifecycle

import (
	"context"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type pathsRuntime struct {
	*countedRuntime
	release, cancelled chan struct{}
}

func (f *pathsRuntime) Paths(ctx context.Context, project, path string, emit func(containerterm.PathListing)) (containerterm.PathListing, error) {
	if project != f.p.Id {
		return containerterm.PathListing{}, status.Error(codes.NotFound, "wrong project")
	}
	if path == "/denied" {
		return containerterm.PathListing{}, status.Error(codes.PermissionDenied, "directory denied")
	}
	out := containerterm.PathListing{Entries: []containerterm.PathEntry{{Name: "한글 dir", Directory: true}}}
	emit(out)
	if path == "~/cancel" {
		<-ctx.Done()
		close(f.cancelled)
		return out, ctx.Err()
	}
	select {
	case <-f.release:
	case <-ctx.Done():
		return out, ctx.Err()
	}
	out.Entries = append(out.Entries, containerterm.PathEntry{Name: "link", Symlink: true, LinkTarget: "한글 dir", Directory: true}, containerterm.PathEntry{Name: "run.sh", Executable: true})
	emit(out)
	out.Truncated = true
	return out, nil
}

func TestProjectPathsRPCStreamsAndCancels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &pathsRuntime{countedRuntime: &countedRuntime{fixture: &fixture{p: &api.Project{Id: "project", Name: "workspace", Workspace: "/workspace", State: "running"}}}, release: make(chan struct{}), cancelled: make(chan struct{})}
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stack.Project().Add(ctx, resource.ProjectAddRequest_builder{Workspace: "/workspace"}.Build()); err != nil {
		t.Fatal(err)
	}
	ln := bufconn.Listen(1 << 20)
	g := grpc.NewServer()
	resource.RegisterServer(g, stack)
	go g.Serve(ln)
	defer g.Stop()
	conn, err := grpc.NewClient("passthrough:///paths", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := resourceclient.New(conn)
	var snapshots []containerterm.PathListing
	out, err := c.Paths(ctx, "project", "~/", func(listing containerterm.PathListing) {
		snapshots = append(snapshots, listing)
		if len(snapshots) == 1 {
			close(f.release) // No final response can arrive until the first batch is visible.
		}
	})
	want := []containerterm.PathEntry{{Name: "한글 dir", Directory: true}, {Name: "link", Symlink: true, LinkTarget: "한글 dir", Directory: true}, {Name: "run.sh", Executable: true}}
	if err != nil || !reflect.DeepEqual(out.Entries, want) || !out.Truncated || len(snapshots) != 3 || len(snapshots[0].Entries) != 1 {
		t.Fatalf("streamed entries: %+v, snapshots %+v, error %v", out, snapshots, err)
	}
	request, abort := context.WithCancel(ctx)
	_, err = c.Paths(request, "project", "~/cancel", func(containerterm.PathListing) { abort() })
	if status.Code(err) != codes.Canceled {
		t.Fatal("client cancellation lost", err)
	}
	select {
	case <-f.cancelled:
	case <-ctx.Done():
		t.Fatal("directory work continued after cancellation")
	}
	for _, tc := range []struct {
		project, path string
		code          codes.Code
	}{
		{"project", "relative", codes.InvalidArgument},
		{"project", "/bad\x00", codes.InvalidArgument},
		{"project", "/" + strings.Repeat("a", 4096), codes.InvalidArgument},
		{"missing", "/", codes.NotFound},
		{"project", "/denied", codes.PermissionDenied},
	} {
		if _, err := c.Paths(ctx, tc.project, tc.path, nil); status.Code(err) != tc.code {
			t.Fatalf("%s %q: %v", tc.project, tc.path, err)
		}
	}
	if f.snapshots.Load() != 0 {
		t.Fatal("path browsing refreshed the full project inventory")
	}
	layer, _ := resource.Find[Layer](stack)
	if _, err := layer.Next().Project().Patch(ctx, resource.ProjectPatchRequest_builder{Ref: projectRef("project"), Listed: ptr(false), DateUpdatedForce: ptr(true)}.Build()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Paths(ctx, "project", "/", nil); status.Code(err) != codes.NotFound {
		t.Fatal("deleted project remained browsable", err)
	}
}
