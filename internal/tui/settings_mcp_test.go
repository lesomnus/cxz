package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type mcpTestClient struct {
	api.SessionsClient
	calls    []string
	projects []string
	enabled  []bool
}

func (c *mcpTestClient) entries() *api.McpServersReply {
	return &api.McpServersReply{Entries: []*api.McpEntry{{
		Id: "A", Server: &api.McpServer{Name: "A", Kind: "stdio", Enabled: true}, Effective: true,
	}}}
}

func (c *mcpTestClient) GetMcpServers(_ context.Context, r *api.McpServersInput, _ ...grpc.CallOption) (*api.McpServersReply, error) {
	c.calls, c.projects = append(c.calls, "get"), append(c.projects, r.Project)
	return c.entries(), nil
}

func (c *mcpTestClient) SetMcpServerDefault(_ context.Context, r *api.McpServerDefaultInput, _ ...grpc.CallOption) (*api.McpServersReply, error) {
	c.calls, c.projects = append(c.calls, "set-default"), append(c.projects, "")
	c.enabled = append(c.enabled, r.Enabled)
	return c.entries(), nil
}

func (c *mcpTestClient) SetProjectMcpServer(_ context.Context, r *api.ProjectMcpServerInput, _ ...grpc.CallOption) (*api.McpServersReply, error) {
	c.calls, c.projects = append(c.calls, "set-project"), append(c.projects, r.Project)
	c.enabled = append(c.enabled, r.Enabled)
	return c.entries(), nil
}

func (c *mcpTestClient) ClearProjectMcpServer(_ context.Context, r *api.ClearProjectMcpServerInput, _ ...grpc.CallOption) (*api.McpServersReply, error) {
	c.calls, c.projects = append(c.calls, "clear-project"), append(c.projects, r.Project)
	return c.entries(), nil
}

func (c *mcpTestClient) McpSessions(_ context.Context, _ *api.McpSessionsInput, _ ...grpc.CallOption) (*api.McpSessionsReply, error) {
	c.calls = append(c.calls, "sessions")
	return &api.McpSessionsReply{Sessions: []*api.McpSessionStatus{{
		SessionId: "S", Title: "a conversation", Pending: true, Servers: map[string]string{"A": "connected"},
	}}}, nil
}

// The page's scope decides which of the three activation calls a keypress is,
// rather than a field left empty deciding it for the server. Toggling in the
// project scope is the project's decision, and i restores the default as its
// own call rather than the same one with the value left out.
func TestMCPSettingsScopesAndInheritance(t *testing.T) {
	m := conversationModel()
	c := &mcpTestClient{}
	m.client = c
	m.project = &api.Project{Id: "P", Name: "Project P"}
	m.settingsPage = &settingsPage{}
	m.Update(m.openMCP()())
	p := m.settingsPage.mcp
	if !p.global || len(p.result.GetEntries()) != 1 {
		t.Fatal(p)
	}
	// The global scope asks about no project, so it does not reach a container.
	if len(c.calls) != 1 || c.calls[0] != "get" || c.projects[0] != "" {
		t.Fatal("the installation's own scope asked a project", c.calls, c.projects)
	}

	m.Update(m.mcpKey(tea.KeyMsg{Type: tea.KeyTab})())
	if p.global || c.projects[1] != "P" {
		t.Fatal(c.calls, c.projects)
	}
	// In a project scope the live sessions are a second question.
	if c.calls[len(c.calls)-1] != "sessions" || len(p.sessions) != 1 {
		t.Fatal("session status was not read beside the registrations", c.calls, p.sessions)
	}

	m.Update(m.mcpKey(tea.KeyMsg{Type: tea.KeyEnter})())
	if got := c.calls[len(c.calls)-2]; got != "set-project" {
		t.Fatal("toggling in a project scope was not the project's decision:", got)
	}
	if c.enabled[len(c.enabled)-1] {
		t.Fatal("toggling an enabled entry did not turn it off")
	}

	m.Update(m.mcpKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})())
	if got := c.calls[len(c.calls)-2]; got != "clear-project" {
		t.Fatal("inherit was not its own call:", got)
	}

	if !strings.Contains(m.mcpScreen(), "Project P") {
		t.Fatal(m.mcpScreen())
	}
	old := p
	m.mcpKey(tea.KeyMsg{Type: tea.KeyEsc})
	m.receiveMCP(mcpResult{page: old, reply: &api.McpServersReply{Message: "stale"}})
	if m.settingsPage.mcp != nil {
		t.Fatal("late reply reopened view")
	}
}

// The global scope has no project to decide for, so toggling there sets the
// installation's default rather than an override on whichever project the page
// happens to be near.
func TestMCPGlobalToggleSetsTheDefault(t *testing.T) {
	m := conversationModel()
	c := &mcpTestClient{}
	m.client = c
	m.project = &api.Project{Id: "P", Name: "Project P"}
	m.settingsPage = &settingsPage{}
	m.Update(m.openMCP()())
	m.Update(m.mcpKey(tea.KeyMsg{Type: tea.KeyEnter})())
	if got := c.calls[len(c.calls)-1]; got != "set-default" {
		t.Fatal("a global toggle was not the installation's default:", got)
	}
	if c.projects[len(c.projects)-1] != "" {
		t.Fatal("a default named a project:", c.projects)
	}
}
