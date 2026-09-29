package tui

import (
	"context"
	"encoding/json"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"google.golang.org/grpc"
	"strings"
	"testing"
)

type mcpTestClient struct {
	api.SessionsClient
	requests []mcpconfig.Request
}

func (c *mcpTestClient) Docker(_ context.Context, r *api.DockerInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	var req mcpconfig.Request
	json.Unmarshal(r.Spec, &req)
	c.requests = append(c.requests, req)
	b, _ := json.Marshal(mcpconfig.Reply{Entries: []mcpconfig.Entry{{ID: "A", Server: mcpconfig.Server{Name: "A", Kind: "stdio", Enabled: true}, Effective: true}}})
	return &api.Receipt{Status: string(b)}, nil
}
func TestMCPSettingsScopesAndInheritance(t *testing.T) {
	m := conversationModel()
	c := &mcpTestClient{}
	m.client = c
	m.project = &api.Project{Id: "P", Name: "Project P"}
	m.settingsPage = &settingsPage{}
	m.Update(m.openMCP()())
	p := m.settingsPage.mcp
	if !p.global || len(p.result.Entries) != 1 {
		t.Fatal(p)
	}
	m.Update(m.mcpKey(tea.KeyMsg{Type: tea.KeyTab})())
	if p.global || c.requests[len(c.requests)-1].Project != "P" {
		t.Fatal(c.requests)
	}
	m.Update(m.mcpKey(tea.KeyMsg{Type: tea.KeyEnter})())
	last := c.requests[len(c.requests)-1]
	if last.Project != "P" || last.Enabled == nil || *last.Enabled {
		t.Fatal(last)
	}
	m.Update(m.mcpKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})())
	last = c.requests[len(c.requests)-1]
	if last.Enabled != nil || last.Action != "enable" {
		t.Fatal(last)
	}
	if !strings.Contains(m.mcpScreen(), "Project P") {
		t.Fatal(m.mcpScreen())
	}
	old := p
	m.mcpKey(tea.KeyMsg{Type: tea.KeyEsc})
	m.receiveMCP(mcpResult{page: old, reply: mcpconfig.Reply{Message: "stale"}})
	if m.settingsPage.mcp != nil {
		t.Fatal("late reply reopened view")
	}
}
