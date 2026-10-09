package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/mcpkind"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mcpFixture struct {
	fixture
	put      *api.PutMcpServerInput
	sessions []string
	ids      []string
}

func (f *mcpFixture) PutMcpServer(_ context.Context, r *api.PutMcpServerInput) (*api.McpServersReply, error) {
	f.put = r
	return &api.McpServersReply{Entries: []*api.McpEntry{{Id: r.Id, Server: r.Server}}}, nil
}

func (f *mcpFixture) GetMcpServers(_ context.Context, _ *api.McpServersInput) (*api.McpServersReply, error) {
	on := true
	return &api.McpServersReply{Entries: []*api.McpEntry{
		{Id: "a", Server: &api.McpServer{Name: "a", Kind: "stdio", Command: "serve"}, Override: &on, Effective: true},
		{Id: "b", Server: &api.McpServer{Name: "b", Kind: "http", Url: "https://example.test/mcp"}},
	}}, nil
}

func (f *mcpFixture) RestartMcp(_ context.Context, r *api.RestartMcpInput) (*api.Receipt, error) {
	f.sessions, f.ids = append(f.sessions, r.SessionId), append(f.ids, r.Id)
	return &api.Receipt{Status: "closed"}, nil
}

func mcpStack(t *testing.T) (*mcpFixture, resource.Server, func()) {
	t.Helper()
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f := &mcpFixture{fixture: fixture{
		p: &api.Project{Id: "project", Workspace: "/work", Name: "work", State: "running"},
		s: &api.Session{
			Id: "session", ProjectId: "project", Workspace: "/work", Title: "a conversation",
			Agent: "claude", Account: "work", AuthBackend: accounts.ProjectLocalOAuth,
			State: "idle", RunId: "run", CreateId: "create",
		},
	}}
	f.s.AuthBinding = accounts.BindingID("project", "work", accounts.ProjectLocalOAuth)
	stack, err := Build(ctx, db, f)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err = stack.Session().List(ctx, &resource.SessionListRequest{}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return f, stack, func() { db.Close() }
}

// The kind is an enum here and a name past this layer. A value this build does
// not know is refused at the boundary rather than stored as a connection it
// cannot make, which is the whole reason the public surface has an enum.
func TestMcpKindIsRefusedAtTheBoundary(t *testing.T) {
	ctx := context.Background()
	f, stack, done := mcpStack(t)
	defer done()

	_, err := stack.Project().PutMcpServer(ctx, resource.PutMcpServerRequest_builder{
		Id: ptr("a"),
		Server: resource.McpServer_builder{
			Name: ptr("a"), Kind: ptr(resource.McpKind_MCP_KIND_UNSPECIFIED),
		}.Build(),
	}.Build())
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal("an unknown connection kind was passed on:", err)
	}
	if f.put != nil {
		t.Fatal("it reached the runtime anyway:", f.put)
	}

	// A definition with no server at all is refused for the same reason: there
	// is nothing to validate and nothing to store.
	if _, err = stack.Project().PutMcpServer(ctx, resource.PutMcpServerRequest_builder{Id: ptr("a")}.Build()); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}

	out, err := stack.Project().PutMcpServer(ctx, resource.PutMcpServerRequest_builder{
		Id: ptr("a"),
		Server: resource.McpServer_builder{
			Name: ptr("a"), Kind: ptr(resource.McpKind_MCP_KIND_STDIO), Command: ptr("serve"),
		}.Build(),
	}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if f.put == nil || f.put.Server.Kind != mcpkind.Stdio {
		t.Fatal("the kind did not arrive as a name:", f.put)
	}
	// And it comes back as the enum, not as whatever string the store holds.
	if len(out.GetEntries()) != 1 || out.GetEntries()[0].GetServer().GetKind() != resource.McpKind_MCP_KIND_STDIO {
		t.Fatal("the reply did not carry the kind back:", out.GetEntries())
	}
}

// Absent means inherited, which is a different answer from switched off, so it
// has to survive the trip out.
func TestMcpOverrideSurvivesTheReply(t *testing.T) {
	ctx := context.Background()
	_, stack, done := mcpStack(t)
	defer done()
	out, err := stack.Project().GetMcpServers(ctx, resource.McpServersRequest_builder{}.Build())
	if err != nil {
		t.Fatal(err)
	}
	var a, b *resource.McpEntry
	for _, e := range out.GetEntries() {
		switch e.GetId() {
		case "a":
			a = e
		case "b":
			b = e
		}
	}
	if a == nil || !a.HasOverride() || !a.GetOverride() {
		t.Fatal("a project's own decision was lost")
	}
	if b == nil || b.HasOverride() {
		t.Fatal("an inherited entry came back as a decision")
	}
}

// A restart is the session's, so it names a session and the ref is resolved
// here. The envelope took a project and a session together and had to check
// that they agreed.
func TestRestartMcpResolvesItsSession(t *testing.T) {
	ctx := context.Background()
	f, stack, done := mcpStack(t)
	defer done()
	out, err := stack.Session().RestartMcp(ctx, resource.SessionMcpRequest_builder{
		Ref: sessionRef("session"), Id: ptr("a"),
	}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.sessions) != 1 || f.sessions[0] != "session" || f.ids[0] != "a" {
		t.Fatal("the runtime was asked about", f.sessions, f.ids)
	}
	if out.GetStatus() != "closed" {
		t.Fatal(out.GetStatus())
	}
	if _, err = stack.Session().RestartMcp(ctx, resource.SessionMcpRequest_builder{
		Ref: sessionRef("nope"), Id: ptr("a"),
	}.Build()); err == nil {
		t.Fatal("accepted a session the installation does not hold")
	}
}
