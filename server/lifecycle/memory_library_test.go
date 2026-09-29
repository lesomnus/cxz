package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/memorylib"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"net"
	"path/filepath"
	"testing"
	"time"
)

type libraryRuntime struct {
	*fixture
	root string
}

func (f *libraryRuntime) Library(ctx context.Context, id string, q memorylib.Request) (memorylib.Reply, error) {
	return memorylib.New(f.root, core.Session{ID: id, ProjectID: f.p.Id, Kind: "codex"}).Do(ctx, q)
}
func TestLibraryRPCUsesResolvedSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, _, e := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	p := &api.Project{Id: core.ID(), Workspace: "/workspace", Name: "project", State: "running"}
	s := &api.Session{Id: core.ID(), ProjectId: p.Id, CreateId: "create", Agent: "codex", Account: "work", AuthBackend: accounts.ProjectLocalOAuth, AuthBinding: accounts.BindingID(p.Id, "work", accounts.ProjectLocalOAuth), State: "idle"}
	f := &libraryRuntime{fixture: &fixture{p: p, s: s}, root: t.TempDir()}
	stack, e := Build(ctx, db, f)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = stack.Project().Add(ctx, resource.ProjectAddRequest_builder{Workspace: p.Workspace}.Build()); e != nil {
		t.Fatal(e)
	}
	if _, e = stack.Project().Up(ctx, resource.ProjectUpRequest_builder{Ref: projectRef(p.Id), ClientId: ptr("up")}.Build()); e != nil {
		t.Fatal(e)
	}
	ln := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	resource.RegisterServer(srv, stack)
	go srv.Serve(ln)
	defer srv.Stop()
	conn, e := grpc.NewClient("passthrough:///memory", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }))
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	client := resourceclient.New(conn)
	if _, e = client.Library(ctx, s.Id, memorylib.Request{Action: "update", Document: "handoff.md", Content: "정리 중"}); e != nil {
		t.Fatal(e)
	}
	out, e := client.Library(ctx, s.Id, memorylib.Request{Action: "read", Document: "handoff.md"})
	if e != nil || out.Content != "정리 중" {
		t.Fatal(out, e)
	}
	if _, e = client.Library(ctx, "missing", memorylib.Request{Action: "list"}); e == nil {
		t.Fatal("unknown session accepted")
	}
}
