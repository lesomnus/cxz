package lifecycle

import (
	"context"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type countedRuntime struct {
	*fixture
	snapshots atomic.Int64
}

func (f *countedRuntime) ResourceSnapshot(ctx context.Context) (*api.ProjectList, *api.SessionList, error) {
	f.snapshots.Add(1)
	return f.fixture.ResourceSnapshot(ctx)
}

func TestResourceReadsAndSubscriptionsDoNotPollRuntime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &countedRuntime{fixture: &fixture{p: &api.Project{Id: "project", Workspace: "/workspace", State: "running"}, s: &api.Session{Id: "session", ProjectId: "project", Agent: "codex", Account: "work", CreateId: "create", RunId: "run", State: "idle"}}}
	f.s.AuthBackend = accounts.ProjectLocalOAuth
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	ln := bufconn.Listen(1 << 20)
	var rpcCount atomic.Int64
	g := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, r any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		rpcCount.Add(1)
		return h(ctx, r)
	}))
	resource.RegisterServer(g, stack)
	go g.Serve(ln)
	defer g.Stop()
	conn, err := grpc.NewClient("passthrough:///test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := resourceclient.New(conn)
	for i := 0; i < 5; i++ {
		before := rpcCount.Load()
		list, err := c.List(ctx, &api.Empty{})
		if err != nil {
			t.Fatal(err)
		}
		if len(list.Sessions) != 1 || list.Sessions[0].Account != "work" || list.Sessions[0].AuthBinding == "" {
			t.Fatal(list)
		}
		if rpcCount.Load()-before != 1 {
			t.Fatal("N+1 relation RPCs", rpcCount.Load()-before)
		}
		if _, err := c.RegisteredProjects(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if f.snapshots.Load() != 1 {
		t.Fatal("runtime reconciliation on reads", f.snapshots.Load())
	}
	changed := make(chan struct{}, 8)
	done := make(chan error, 1)
	go func() {
		done <- c.WatchChanges(ctx, []string{f.p.Id}, []string{f.s.Id}, func() { changed <- struct{}{} })
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-changed:
		case err := <-done:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	before := rpcCount.Load()
	select {
	case <-changed:
		t.Fatal("idle stream generated change")
	case <-time.After(1100 * time.Millisecond):
	}
	if f.snapshots.Load() != 1 || rpcCount.Load() != before {
		t.Fatal("idle polling")
	}
	layer, _ := resource.Find[Layer](stack)
	if _, err := layer.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef(f.s.Id), Name: ptr("renamed"), DateUpdatedForce: ptr(true)}.Build()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case err := <-done:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch cancellation leaked")
	}
}
