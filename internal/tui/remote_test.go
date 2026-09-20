package tui

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/transport"
)

func TestRemoteWorkspaceAndLocalDockerGuard(t *testing.T) {
	ctx := transport.WithRemote(context.Background())
	if got, err := workspacePath(ctx, "/srv/work/../project"); err != nil || got != "/srv/project" {
		t.Fatal(got, err)
	}
	for _, path := range []string{".", "relative", `C:\work`, "~/work"} {
		if _, err := workspacePath(ctx, path); err == nil {
			t.Fatal("local path accepted", path)
		}
	}
	if _, err := containerterm.Open(ctx, nil, 80, 24, func() {}); err == nil {
		t.Fatal("remote terminal reached local Docker")
	}
	if _, err := containerterm.ListPaths(ctx, nil, "/"); err == nil {
		t.Fatal("remote lookup reached local Docker")
	}
}

type connectionContextKey struct{}
type routingUIClient struct {
	api.SessionsClient
	calls    []string
	accounts map[string]*accountViewService
}

func (c *routingUIClient) ContextFor(ctx context.Context, ref string) context.Context {
	return context.WithValue(ctx, connectionContextKey{}, c.ConnectionName(ref))
}
func (c *routingUIClient) ConnectionName(ref string) string {
	name, _, _ := strings.Cut(ref, "::")
	return name
}
func (c *routingUIClient) DefaultConnection() string { return "work" }
func (c *routingUIClient) AccountClient(ref string) resource.AccountServiceClient {
	return c.accounts[c.ConnectionName(ref)]
}
func (c *routingUIClient) Docker(ctx context.Context, _ *api.DockerInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.calls = append(c.calls, ctx.Value(connectionContextKey{}).(string))
	return &api.Receipt{Status: `{"mode":"dind","state":"running"}`}, nil
}
func TestConnectionPanelFocusAndOverlayRouting(t *testing.T) {
	m := conversationModel()
	c := &routingUIClient{accounts: map[string]*accountViewService{"home": {}, "work": {}}}
	m.client = c
	m.initializeNavigation(nil, "")
	listing := listing{projectsLoaded: true, projects: []*api.Project{
		{Id: "home::p", Name: "project1 via home"}, {Id: "work::p", Name: "project1 via work"},
	}, sessions: []*api.Session{{Id: "home::s", ProjectId: "home::p"}, {Id: "work::s", ProjectId: "work::p"}}}
	m.updatePanel(listing)
	if m.panelRows()[m.panelIndex].project.Id != "work::p" {
		t.Fatal("default focus lost")
	}
	m.sessions = listing.sessions[:1] // Active conversation is home; selected panel row is work.
	cmd := m.openSettings()
	m.panelIndex = 0 // Overlay must retain work even if selection changes before the RPC runs.
	m.Update(cmd())
	if len(c.calls) != 1 || c.calls[0] != "work" || !strings.Contains(m.settingsScreen(), "via work") {
		t.Fatal(c.calls, m.settingsScreen())
	}
	m.settingsPage = nil
	m.panelIndex = 2
	accountCmd := m.openAccounts(false)
	m.panelIndex = 0
	if m.accountClient() != c.accounts["work"] {
		t.Fatal("accounts routed to current row instead of captured connection")
	}
	first := m.accountRequest
	m.accountView = false
	m.openAccounts(false) // New view targets home.
	m.Update(accountCmd())
	if m.accountRequest == first || !m.accountLoading {
		t.Fatal("stale account load replaced new view")
	}
}

func TestNewSessionLeavesEmptyConnectionPlaceholder(t *testing.T) {
	m := conversationModel()
	m.project = &api.Project{Id: "work::@connection", State: "connection"}
	m.wantID = "work::new"
	m.Update(listing{projectsLoaded: true, projects: []*api.Project{{Id: "work::p", Name: "new project via work", Workspace: "/workspace"}}, sessions: []*api.Session{{Id: "work::new", ProjectId: "work::p", Workspace: "/workspace"}}})
	if m.wantID != "" || m.current() == nil || m.current().Id != "work::new" || m.project.Id != "work::p" {
		t.Fatal("new project session not selected")
	}
}
