package multiclient

import (
	"context"
	"testing"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mcpDaemon struct {
	daemon
	project string
	session string
	id      string
	calls   []string
}

func (d *mcpDaemon) GetMcpServers(_ context.Context, r *api.McpServersInput, _ ...grpc.CallOption) (*api.McpServersReply, error) {
	d.calls, d.project = append(d.calls, "get"), r.Project
	return &api.McpServersReply{}, nil
}

func (d *mcpDaemon) SetProjectMcpServer(_ context.Context, r *api.ProjectMcpServerInput, _ ...grpc.CallOption) (*api.McpServersReply, error) {
	d.calls = append(d.calls, "set-project")
	d.project, d.id = r.Project, r.Id
	return &api.McpServersReply{}, nil
}

func (d *mcpDaemon) RestartMcp(_ context.Context, r *api.RestartMcpInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	d.calls = append(d.calls, "restart")
	d.session, d.id = r.SessionId, r.Id
	return &api.Receipt{}, nil
}

func (d *mcpDaemon) McpSessions(_ context.Context, r *api.McpSessionsInput, _ ...grpc.CallOption) (*api.McpSessionsReply, error) {
	d.calls, d.project = append(d.calls, "sessions"), r.Project
	return &api.McpSessionsReply{Sessions: []*api.McpSessionStatus{{SessionId: "S"}}}, nil
}

// The project says which installation holds the registrations, read from a
// field. It used to be parsed out of the payload, and the payload written back
// out with the prefix stripped; the caller's own request is left intact.
func TestMcpRoutesByProject(t *testing.T) {
	a, b := &mcpDaemon{}, &mcpDaemon{}
	c := New(t.Context(), []Source{{Name: "a", Client: a}, {Name: "b", Client: b}}, "a")
	defer c.Close()
	in := &api.ProjectMcpServerInput{Project: "b::P", Id: "a", Enabled: true}
	if _, err := c.SetProjectMcpServer(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	if len(a.calls) != 0 || b.project != "P" || b.id != "a" {
		t.Fatal("wrong installation or project", a.calls, b.project, b.id)
	}
	if in.Project != "b::P" {
		t.Fatal("the caller's request was mutated:", in.Project)
	}
}

// A restart names a session and nothing else, and a session names the
// connection that holds it. The envelope carried a project and a session
// together and had to refuse the pairs that disagreed; there is no pair now.
func TestRestartMcpFollowsItsSession(t *testing.T) {
	a, b := &mcpDaemon{}, &mcpDaemon{}
	c := New(t.Context(), []Source{{Name: "a", Client: a}, {Name: "b", Client: b}}, "a")
	defer c.Close()
	// Selected connection a, session on b: the session decides, and that is
	// no longer a conflict to report.
	ctx := c.ContextFor(t.Context(), "a::P")
	if _, err := c.RestartMcp(ctx, &api.RestartMcpInput{SessionId: "b::S", Id: "x"}); err != nil {
		t.Fatal(err)
	}
	if len(a.calls) != 0 || b.session != "S" || b.id != "x" {
		t.Fatal("a restart did not follow its session", a.calls, b.session, b.id)
	}
}

// Session ids come back prefixed, as they do everywhere else: a caller that
// reads one out of a status and then asks about it must not be handed the one
// id it cannot use.
func TestMcpSessionStatusCarriesPrefixedIds(t *testing.T) {
	a, b := &mcpDaemon{}, &mcpDaemon{}
	c := New(t.Context(), []Source{{Name: "a", Client: a}, {Name: "b", Client: b}}, "a")
	defer c.Close()
	out, err := c.McpSessions(t.Context(), &api.McpSessionsInput{Project: "b::P"})
	if err != nil {
		t.Fatal(err)
	}
	if b.project != "P" || len(out.Sessions) != 1 || out.Sessions[0].SessionId != "b::S" {
		t.Fatal("status did not come back addressable", b.project, out.Sessions)
	}
}

// Delivery runs from a manager to a project container, so a frontend
// connection says so rather than reaching for a manager that would deliver to
// itself.
func TestSyncMcpServersIsNotAFrontendCall(t *testing.T) {
	c := New(t.Context(), []Source{{Name: "a", Client: &mcpDaemon{}}}, "a")
	defer c.Close()
	if _, err := c.SyncMcpServers(t.Context(), &api.SyncMcpServersInput{}); status.Code(err) != codes.Unimplemented {
		t.Fatal(err)
	}
}
