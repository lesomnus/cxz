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

// libraryStack is the same wiring as above, returned so a test can speak the
// typed API directly rather than through the view model.
func libraryStack(t *testing.T) (resource.SessionServiceClient, *api.Session, func()) {
	t.Helper()
	ctx := context.Background()
	db, _, e := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if e != nil {
		t.Fatal(e)
	}
	p := &api.Project{Id: core.ID(), Workspace: "/workspace", Name: "project", State: "running"}
	s := &api.Session{Id: core.ID(), ProjectId: p.Id, CreateId: "create", Agent: "codex", Account: "work", AuthBackend: accounts.ProjectLocalOAuth, AuthBinding: accounts.BindingID(p.Id, "work", accounts.ProjectLocalOAuth), State: "idle"}
	f := &libraryRuntime{fixture: &fixture{p: p, s: s}, root: t.TempDir()}
	stack, e := Build(ctx, db, f)
	if e != nil {
		db.Close()
		t.Fatal(e)
	}
	if _, e = stack.Project().Add(ctx, resource.ProjectAddRequest_builder{Workspace: p.Workspace}.Build()); e != nil {
		db.Close()
		t.Fatal(e)
	}
	if _, e = stack.Project().Up(ctx, resource.ProjectUpRequest_builder{Ref: projectRef(p.Id), ClientId: ptr("up")}.Build()); e != nil {
		db.Close()
		t.Fatal(e)
	}
	ln := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	resource.RegisterServer(srv, stack)
	go srv.Serve(ln)
	conn, e := grpc.NewClient("passthrough:///memory", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }))
	if e != nil {
		srv.Stop()
		db.Close()
		t.Fatal(e)
	}
	return resource.NewSessionServiceClient(conn), s, func() {
		conn.Close()
		srv.Stop()
		db.Close()
	}
}

// The library's operations are methods, so each one carries only what it can
// mean something by. The envelope took a scope and a memory id whatever was
// asked and then refused most of the combinations.
func TestLibraryOperationsCarryOnlyTheirOwnArguments(t *testing.T) {
	ctx := context.Background()
	client, s, done := libraryStack(t)
	defer done()
	ref := sessionRef(s.Id)

	// Writing names no memory and no scope: only this session's own memory can
	// be written, and never one in the trash.
	wrote, e := client.WriteLibraryDocument(ctx, resource.SessionLibraryWriteRequest_builder{
		Ref: ref, Document: ptr("handoff.md"), Content: ptr("first"),
	}.Build())
	if e != nil {
		t.Fatal(e)
	}
	if wrote.GetRevision() == "" {
		t.Fatal("a write did not report the revision to present next time")
	}

	// Reading a memory and reading what it has deleted are a selector, and the
	// memory comes back either way.
	present, e := client.LibraryMemory(ctx, resource.SessionLibraryMemoryRequest_builder{
		Ref: ref, Documents: ptr(resource.LibraryDocuments_LIBRARY_DOCUMENTS_PRESENT),
	}.Build())
	if e != nil {
		t.Fatal(e)
	}
	if len(present.GetDocuments()) != 1 || present.GetDocuments()[0].GetName() != "handoff.md" {
		t.Fatalf("%+v", present.GetDocuments())
	}
	if present.GetMemory().GetCreated() == nil {
		t.Fatal("a memory came back without the time it was created")
	}

	// Deleting a document takes the revision that was read, so a stale view
	// cannot remove it.
	if _, e = client.DeleteLibraryDocument(ctx, resource.SessionLibraryDocumentEditRequest_builder{
		Ref: ref, Document: ptr("handoff.md"), Revision: ptr("stale"),
	}.Build()); e == nil {
		t.Fatal("deleted a document against a revision nobody read")
	}
	if _, e = client.DeleteLibraryDocument(ctx, resource.SessionLibraryDocumentEditRequest_builder{
		Ref: ref, Document: ptr("handoff.md"), Revision: ptr(wrote.GetRevision()),
	}.Build()); e != nil {
		t.Fatal(e)
	}
	deleted, e := client.LibraryMemory(ctx, resource.SessionLibraryMemoryRequest_builder{
		Ref: ref, Documents: ptr(resource.LibraryDocuments_LIBRARY_DOCUMENTS_DELETED),
	}.Build())
	if e != nil {
		t.Fatal(e)
	}
	if len(deleted.GetDocuments()) != 1 {
		t.Fatalf("the deleted document is not listed as deleted: %+v", deleted.GetDocuments())
	}

	// A scope this build does not know is refused rather than read as the
	// library when the trash was meant.
	if _, e = client.LibraryMemories(ctx, resource.SessionLibraryMemoriesRequest_builder{
		Ref: ref, Scope: ptr(resource.LibraryScope(99)),
	}.Build()); e == nil {
		t.Fatal("accepted a scope it cannot resolve")
	}
}

// A listing and a search are one method: a query filters the listing rather
// than asking a different question.
func TestLibraryQueryFiltersTheListing(t *testing.T) {
	ctx := context.Background()
	client, s, done := libraryStack(t)
	defer done()
	ref := sessionRef(s.Id)
	if _, e := client.WriteLibraryDocument(ctx, resource.SessionLibraryWriteRequest_builder{
		Ref: ref, Document: ptr("handoff.md"), Content: ptr("salmon"),
	}.Build()); e != nil {
		t.Fatal(e)
	}
	all, e := client.LibraryMemories(ctx, resource.SessionLibraryMemoriesRequest_builder{Ref: ref}.Build())
	if e != nil || len(all.GetMemories()) != 1 {
		t.Fatal(all.GetMemories(), e)
	}
	hit, e := client.LibraryMemories(ctx, resource.SessionLibraryMemoriesRequest_builder{
		Ref: ref, Query: ptr("salmon"),
	}.Build())
	if e != nil || len(hit.GetMemories()) != 1 {
		t.Fatal("the query did not find the document text", hit.GetMemories(), e)
	}
	miss, e := client.LibraryMemories(ctx, resource.SessionLibraryMemoriesRequest_builder{
		Ref: ref, Query: ptr("halibut"),
	}.Build())
	if e != nil || len(miss.GetMemories()) != 0 {
		t.Fatal("the query matched everything", miss.GetMemories(), e)
	}
}
