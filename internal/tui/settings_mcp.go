package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"strings"
	"time"
)

type mcpPage struct {
	project, projectName string
	global               bool
	selected, offset     int
	busy                 bool
	result               *api.McpServersReply
	sessions             []*api.McpSessionStatus
	message              string
	request              uint64
	adding               bool
	field                int
	inputs               []textinput.Model
}
type mcpResult struct {
	page     *mcpPage
	request  uint64
	reply    *api.McpServersReply
	sessions []*api.McpSessionStatus
	err      error
}

func (m *model) openMCP() tea.Cmd {
	p := &mcpPage{global: true}
	if m.project != nil {
		p.project = m.project.Id
		p.projectName = m.project.Name
	} else if s := m.current(); s != nil {
		p.project = s.ProjectId
		p.projectName = s.ProjectName
	}
	m.settingsPage.mcp = p
	return m.mcpList()
}

func (m *model) mcpList() tea.Cmd {
	return m.mcpCall(func(ctx context.Context, client api.SessionsClient, project string) (*api.McpServersReply, error) {
		return client.GetMcpServers(ctx, &api.McpServersInput{Project: project})
	})
}

// mcpCall runs one named call and, when the page is looking at a project, also
// asks what that project's live sessions launched with. They are two questions
// now: reading the registrations no longer waits on a container.
func (m *model) mcpCall(call func(context.Context, api.SessionsClient, string) (*api.McpServersReply, error)) tea.Cmd {
	p := m.settingsPage.mcp
	if p.busy {
		return nil
	}
	project := ""
	if !p.global {
		project = p.project
	}
	p.busy = true
	p.request++
	seq := p.request
	ctx, client := m.contextFor(m.settingsPage.connection), m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		out, e := call(ctx, client, project)
		v := mcpResult{page: p, request: seq, reply: out, err: e}
		if e == nil && project != "" {
			// A status that cannot be read is not a failed change: the change
			// is already saved, and the reply says so.
			if st, err := client.McpSessions(ctx, &api.McpSessionsInput{Project: project}); err == nil {
				v.sessions = st.Sessions
			}
		}
		return v
	}
}
func (m *model) receiveMCP(v mcpResult) {
	if m.settingsPage == nil || m.settingsPage.mcp != v.page || v.page.request != v.request {
		return
	}
	p := v.page
	p.busy = false
	if v.err != nil {
		p.message = v.err.Error()
		return
	}
	p.result = v.reply
	p.sessions = v.sessions
	p.message = v.reply.GetMessage()
	p.selected = max(0, min(p.selected, len(p.result.GetEntries())-1))
	p.adding = false
	p.inputs = nil
}

var mcpFields = []string{"ID", "Name", "Type (stdio/http)", "Command or URL", "Arguments (JSON array)", "Environment (JSON object)", "HTTP headers (JSON object)"}

func (m *model) mcpKey(k tea.KeyMsg) tea.Cmd {
	p := m.settingsPage.mcp
	if k.String() == "ctrl+d" && !k.Paste {
		return tea.Quit
	}
	if p.adding {
		if k.String() == "esc" && !k.Paste {
			p.adding = false
			return nil
		}
		if p.busy {
			return nil
		}
		if !k.Paste && (k.String() == "tab" || k.String() == "shift+tab" || k.String() == "enter") {
			if k.String() == "enter" && p.field == len(p.inputs)-1 {
				return m.mcpAdd()
			}
			p.inputs[p.field].Blur()
			d := 1
			if k.String() == "shift+tab" {
				d = -1
			}
			p.field = (p.field + d + len(p.inputs)) % len(p.inputs)
			return p.inputs[p.field].Focus()
		}
		var cmd tea.Cmd
		p.inputs[p.field], cmd = p.inputs[p.field].Update(k)
		return cmd
	}
	if k.Paste {
		return nil
	}
	if k.String() == "esc" {
		m.settingsPage.mcp = nil
		return nil
	}
	if p.busy {
		return nil
	}
	switch k.String() {
	case "tab":
		if p.project != "" {
			p.global = !p.global
			p.selected = 0
			p.offset = 0
			return m.mcpList()
		}
	case "up":
		p.selected = max(0, p.selected-1)
	case "down":
		p.selected = min(len(p.result.GetEntries())-1, p.selected+1)
	case "r":
		return m.mcpList()
	case "a":
		if !p.global {
			p.message = "Register MCP servers in Global defaults (Tab)."
			return nil
		}
		p.adding = true
		p.field = 0
		p.inputs = nil
		for i := range mcpFields {
			v := textinput.New()
			v.CharLimit = 16384
			v.Width = max(10, m.settingsWidth()-8)
			if i == 2 {
				v.SetValue("stdio")
			}
			if i == 4 {
				v.SetValue("[]")
			}
			if i >= 5 {
				v.SetValue("{}")
				v.EchoMode = textinput.EchoPassword
			}
			p.inputs = append(p.inputs, v)
		}
		return p.inputs[0].Focus()
	case "enter", " ", "i":
		if p.selected < 0 || p.selected >= len(p.result.GetEntries()) {
			return nil
		}
		v := p.result.GetEntries()[p.selected]
		// Which of the three activation calls this is depends on the scope the
		// page is in, which the page knows. Nothing has to be inferred from a
		// field being empty.
		if k.String() == "i" {
			if p.global {
				return nil
			}
			return m.mcpCall(func(ctx context.Context, client api.SessionsClient, project string) (*api.McpServersReply, error) {
				return client.ClearProjectMcpServer(ctx, &api.ClearProjectMcpServerInput{Project: project, Id: v.Id})
			})
		}
		on := !v.Effective
		if p.global {
			return m.mcpCall(func(ctx context.Context, client api.SessionsClient, _ string) (*api.McpServersReply, error) {
				return client.SetMcpServerDefault(ctx, &api.McpServerDefaultInput{Id: v.Id, Enabled: on})
			})
		}
		return m.mcpCall(func(ctx context.Context, client api.SessionsClient, project string) (*api.McpServersReply, error) {
			return client.SetProjectMcpServer(ctx, &api.ProjectMcpServerInput{Project: project, Id: v.Id, Enabled: on})
		})
	}
	return nil
}
func (m *model) mcpAdd() tea.Cmd {
	p := m.settingsPage.mcp
	v := func(i int) string { return strings.TrimSpace(p.inputs[i].Value()) }
	s := &api.McpServer{Name: v(1), Kind: v(2)}
	if s.Kind == "http" {
		s.Url = v(3)
	} else {
		s.Command = v(3)
	}
	for i, d := range []any{&s.Args, &s.Env, &s.Headers} {
		if e := json.Unmarshal([]byte(v(i+4)), d); e != nil {
			p.message = mcpFields[i+4] + ": invalid JSON"
			return nil
		}
	}
	if e := mcpconfig.ValidateID(v(0)); e != nil {
		p.message = e.Error()
		return nil
	}
	if e := (mcpconfig.Server{
		Name: s.Name, Kind: s.Kind, Command: s.Command, Args: s.Args,
		Env: s.Env, URL: s.Url, Headers: s.Headers,
	}).Validate(); e != nil {
		p.message = e.Error()
		return nil
	}
	for _, entry := range p.result.GetEntries() {
		if entry.Id == v(0) {
			p.message = "ID already exists; use a new ID or cxz mcp add to replace it."
			return nil
		}
	}
	id := v(0)
	return m.mcpCall(func(ctx context.Context, client api.SessionsClient, _ string) (*api.McpServersReply, error) {
		return client.PutMcpServer(ctx, &api.PutMcpServerInput{Id: id, Server: s})
	})
}
func (m *model) mcpMouse(v tea.MouseMsg) tea.Cmd {
	p := m.settingsPage.mcp
	if p.adding || p.busy {
		return nil
	}
	if v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress {
		i := v.Y - 4 + p.offset
		if i >= 0 && i < len(p.result.GetEntries()) {
			p.selected = i
			return m.mcpKey(tea.KeyMsg{Type: tea.KeyEnter})
		}
	}
	return nil
}
func (m *model) mcpScreen() string {
	p := m.settingsPage.mcp
	w := m.settingsWidth()
	inner := max(1, w-4)
	scope := "Global defaults"
	if !p.global {
		scope = "Project: " + p.projectName
	}
	lines := []string{"", accent.Bold(true).Render("MCP · " + scope + m.connectionLabel(m.settingsPage.connection)), ""}
	if p.adding {
		start := max(0, p.field-max(1, (m.height-8)/2)+1)
		for i := start; i < min(len(mcpFields), start+max(1, (m.height-8)/2)); i++ {
			lines = append(lines, mcpFields[i], p.inputs[i].View())
		}
		lines = append(lines, "New registrations start disabled. Tab next · Enter on last field saves · Esc cancel")
	} else {
		lines = append(lines, "  Enabled  Server / type / activation source")
		visible := max(1, m.height-10)
		p.offset = max(0, min(p.offset, p.selected))
		p.offset = max(p.offset, p.selected-visible+1)
		for i := p.offset; i < min(len(p.result.GetEntries()), p.offset+visible); i++ {
			v := p.result.GetEntries()[i]
			check := "[ ]"
			if v.Effective {
				check = "[✓]"
			}
			source := "default"
			if !p.global && v.Override != nil {
				source = "project override"
			}
			line := fmt.Sprintf("  %s  %s · %s · %s", check, v.GetServer().GetName(), v.GetServer().GetKind(), source)
			if i == p.selected {
				line = focus.Render("›" + line[1:])
			}
			lines = append(lines, line)
		}
		if len(p.result.GetEntries()) == 0 {
			lines = append(lines, "No MCP servers registered. Press a to add one.")
		}
		lines = append(lines, "", "Tab global/project · Enter/Space toggle · i inherit · a add · r refresh · Esc back", "Changes apply on the next agent launch; working agents are not restarted.")
	}
	for i, session := range p.sessions {
		if i >= 3 {
			lines = append(lines, fmt.Sprintf("%d more sessions (cxz mcp list --project …)", len(p.sessions)-i))
			break
		}
		label := "Applied"
		if session.Pending {
			label = "Settings pending restart"
		}
		if p.selected >= 0 && p.selected < len(p.result.GetEntries()) {
			status := session.Servers[p.result.GetEntries()[p.selected].Id]
			if status == "" {
				status = "not in this launch"
			}
			label += " · " + status
		}
		lines = append(lines, safeText(session.Title)+": "+label)
	}
	if p.busy {
		lines = append(lines, "Loading…")
	}
	if p.message != "" {
		lines = append(lines, safeText(p.message))
	}
	for i := range lines {
		lines[i] = "  " + clip(lines[i], inner)
	}
	return screen(strings.Join(lines, "\n"), w, m.height)
}
