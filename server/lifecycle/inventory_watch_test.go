package lifecycle

import (
	"bytes"
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestUnscopedInventoryListAndWatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &fixture{p: &api.Project{Id: "first", Workspace: "/first", State: "running"}, s: &api.Session{Id: "first-session", ProjectId: "first", Agent: "codex", State: "idle"}}
	f.s.Account = "work"
	f.s.AuthBackend = accounts.ProjectLocalOAuth
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	// Initialize the runtime snapshot, then seed another project and an archived
	// resource through the projection layer, without starting real agents.
	if _, err := stack.Project().List(ctx, &resource.ProjectListRequest{}); err != nil {
		t.Fatal(err)
	}
	layer, _ := resource.Find[Layer](stack)
	_, err = layer.Next().Project().Add(ctx, resource.ProjectAddRequest_builder{RuntimeId: "second", Workspace: "/second", Listed: ptr(true)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	seedSession := func(id string) (*resource.Session, error) {
		return layer.saveSession(ctx, &api.Session{Id: id, ProjectId: "second", Account: "work", Agent: "codex", State: "idle", AuthBackend: accounts.ProjectLocalOAuth, AuthBinding: accounts.BindingID("second", "work", accounts.ProjectLocalOAuth)}, "")
	}
	live, err := seedSession("second-session")
	if err != nil {
		t.Fatal(err)
	}
	archived, err := seedSession("archived-session")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := layer.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: resource.SessionRef_builder{Id: archived.GetId()}.Build(), Listed: ptr(false), DateUpdatedForce: ptr(true)}.Build()); err != nil {
		t.Fatal(err)
	}

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
	projects := resource.NewProjectServiceClient(conn)
	sessions := resource.NewSessionServiceClient(conn)
	ps, err := projects.List(ctx, &resource.ProjectListRequest{})
	if err != nil || len(ps.GetItems()) != 2 {
		t.Fatalf("all listed projects: %v %v", ps, err)
	}
	ss, err := sessions.List(ctx, &resource.SessionListRequest{})
	if err != nil || len(ss.GetItems()) != 2 {
		t.Fatalf("sessions across both projects: %v %v", ss, err)
	}
	pw, err := projects.Watch(ctx, &resource.ProjectWatchRequest{})
	if err != nil {
		t.Fatal(err)
	}
	pb, err := pw.Recv()
	if err != nil || len(pb.GetItems()) != 2 {
		t.Fatalf("unscoped project snapshot: %v %v", pb, err)
	}
	sw, err := sessions.Watch(ctx, &resource.SessionWatchRequest{})
	if err != nil {
		t.Fatal(err)
	}
	sb, err := sw.Recv()
	if err != nil || len(sb.GetItems()) != 2 {
		t.Fatalf("unscoped session snapshot: %v %v", sb, err)
	}
	for _, item := range sb.GetItems() {
		if string(item.GetId()) == string(archived.GetId()) {
			t.Fatal("default watch exposed archived session")
		}
	}
	added, err := seedSession("new-session")
	if err != nil {
		t.Fatal(err)
	}
	sb, err = sw.Recv()
	if err != nil || len(sb.GetItems()) != 1 || !bytes.Equal(sb.GetItems()[0].GetId(), added.GetId()) {
		t.Fatalf("new session after subscription: %v %v", sb, err)
	}
	addedProject, err := layer.Next().Project().Add(ctx, resource.ProjectAddRequest_builder{RuntimeId: "third", Workspace: "/third", Listed: ptr(true)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	pb, err = pw.Recv()
	if err != nil || len(pb.GetItems()) != 1 || !bytes.Equal(pb.GetItems()[0].GetId(), addedProject.GetId()) {
		t.Fatalf("new project after subscription: %v %v", pb, err)
	}
	if _, err := layer.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: resource.SessionRef_builder{Id: live.GetId()}.Build(), Name: ptr("renamed"), DateUpdatedForce: ptr(true)}.Build()); err != nil {
		t.Fatal(err)
	}
	sb, err = sw.Recv()
	if err != nil || len(sb.GetItems()) != 1 || sb.GetItems()[0].GetValue().GetName() != "renamed" {
		t.Fatalf("live change in second project: %v %v", sb, err)
	}
	if _, err := layer.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: resource.SessionRef_builder{Id: live.GetId()}.Build(), Listed: ptr(false), DateUpdatedForce: ptr(true)}.Build()); err != nil {
		t.Fatal(err)
	}
	sb, err = sw.Recv()
	if err != nil || len(sb.GetItems()) != 1 || sb.GetItems()[0].GetValue() != nil {
		t.Fatalf("removal from default inventory: %v %v", sb, err)
	}
	// Explicit archived/narrow scopes continue to work.
	aw, err := sessions.Watch(ctx, resource.SessionWatchRequest_builder{Filters: []*resource.SessionFilter{resource.SessionFilter_builder{Ref: resource.SessionRef_builder{Id: archived.GetId()}.Build(), Listed: ptr(false)}.Build()}}.Build())
	if err != nil {
		t.Fatal(err)
	}
	ab, err := aw.Recv()
	if err != nil || len(ab.GetItems()) != 1 || string(ab.GetItems()[0].GetId()) != string(archived.GetId()) {
		t.Fatalf("explicit scope: %v %v", ab, err)
	}
}
