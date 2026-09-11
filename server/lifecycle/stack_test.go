package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"net"
	"path/filepath"
	"testing"
	"time"
)

type fixture struct {
	api.UnimplementedSessionsServer
	p      *api.Project
	s      *api.Session
	resume *api.Control
	up     *api.ProjectRequest
}

func (f *fixture) RegisterProject(context.Context, string, string) (*api.Project, error) {
	return f.p, nil
}
func (f *fixture) ResourceSnapshot(context.Context) (*api.ProjectList, *api.SessionList, error) {
	return &api.ProjectList{Projects: []*api.Project{f.p}}, &api.SessionList{Sessions: []*api.Session{f.s}}, nil
}
func (f *fixture) Resume(_ context.Context, r *api.Control) (*api.Session, error) {
	f.resume = r
	if r.RunId != f.s.RunId {
		return nil, status.Error(codes.FailedPrecondition, "stale run")
	}
	return f.s, nil
}
func (f *fixture) Open(_ context.Context, r *api.ProjectRequest) (*api.Session, error) {
	f.up = r
	return f.s, nil
}

func TestResourceStack(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db") + "?_pragma=foreign_keys(1)", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &fixture{p: &api.Project{Id: "111111111111111111111111", Workspace: "/workspaces/test", Name: "test", State: "running"}, s: &api.Session{Id: "222222222222222222222222", ProjectId: "111111111111111111111111", Workspace: "/workspaces/test", Title: "conversation", CreateId: "create-1", CreatedAt: time.Now().UnixMilli(), Agent: "codex", RunId: "run-1", State: "idle", LastSeq: 1}}
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	p, err := stack.Project().Add(ctx, resource.ProjectAddRequest_builder{Workspace: f.p.Workspace}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.GetId()) != 16 || p.GetId()[9] != 7 {
		t.Fatalf("not a payday Project ID: %x", p.GetId())
	}
	_, err = stack.Project().Up(ctx, resource.ProjectUpRequest_builder{Ref: resource.ProjectRef_builder{Id: p.GetId()}.Build(), ClientId: ptr("up-1")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if f.up == nil || !f.up.PrepareOnly {
		t.Fatal("Project.Up must not create a session")
	}
	s, err := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: resource.SessionRef_builder{ClientId: ptr("create-1")}.Build(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.GetId()) != 16 || s.GetId()[9] != 8 || s.GetRuntimeId() != f.s.Id {
		t.Fatal("session ID mapping lost")
	}
	_, err = stack.Session().Resume(ctx, resource.SessionControl_builder{Ref: resource.SessionRef_builder{Id: s.GetId()}.Build(), RunId: ptr("wrong"), ClientId: ptr("resume-1")}.Build())
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("stale run accepted: %v", err)
	}
	_, err = stack.Session().Resume(ctx, resource.SessionControl_builder{Ref: resource.SessionRef_builder{Id: s.GetId()}.Build(), RunId: ptr("run-1"), ClientId: ptr("resume-2")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if f.resume.SessionId != f.s.Id {
		t.Fatal("runtime ID not resolved")
	}
	_, err = stack.Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef(f.s.Id), Status: resource.SessionStatus_builder{State: "idle"}.Build()}.Build())
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("status mutation bypass: %v", err)
	}
	var audits, tenants int
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM audit").Scan(&audits); err != nil || audits < 2 {
		t.Fatalf("payday audit recorder: %d %v", audits, err)
	}
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM tenant").Scan(&tenants); err != nil || tenants != 0 {
		t.Fatalf("global resources need no tenant: %d %v", tenants, err)
	}
	// Exercise the generated gRPC Watch stream, not the journal Events adapter.
	ln := bufconn.Listen(1 << 20)
	g := grpc.NewServer()
	resource.RegisterServer(g, stack)
	go g.Serve(ln)
	defer g.Stop()
	conn, err := grpc.NewClient("passthrough:///test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := resource.NewSessionServiceClient(conn)
	stream, err := client.Watch(ctx, resource.SessionWatchRequest_builder{Filters: []*resource.SessionFilter{resource.SessionFilter_builder{Ref: sessionRef(f.s.Id)}.Build()}}.Build())
	if err != nil {
		t.Fatal(err)
	}
	first, err := stream.Recv()
	if err != nil || len(first.GetItems()) != 1 {
		t.Fatalf("watch snapshot: %v %v", first, err)
	}
	// Write through the generated Sink and recorder to prove update publication.
	layer, _ := resource.Find[Layer](stack)
	_, err = layer.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef(f.s.Id), Status: resource.SessionStatus_builder{State: "working", LastSeq: 2}.Build(), DateUpdatedForce: ptr(true)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	next, err := stream.Recv()
	if err != nil || len(next.GetItems()) != 1 {
		t.Fatalf("watch update: %v %v", next, err)
	}
}
