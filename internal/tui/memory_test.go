package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/memoryview"
	"google.golang.org/grpc"
)

type memoryClient struct {
	api.SessionsClient
	reads  []*api.MemoryRequest
	copies []*api.CopyMemoryRequest
}

func (c *memoryClient) Memory(_ context.Context, r *api.MemoryRequest, _ ...grpc.CallOption) (*api.MemoryReply, error) {
	c.reads = append(c.reads, r)
	p := memoryview.Page{Path: r.Path, Location: "volume:retained/session-profiles/config/" + r.Path}
	switch r.Path {
	case "":
		p.Directory = true
		p.Entries = []memoryview.Entry{{Name: "projects", Directory: true}}
	case "projects":
		p.Directory = true
		p.Entries = []memoryview.Entry{{Name: "MEMORY.md", Size: 20}}
	default:
		p.Content = strings.Repeat("saved memory\n", 50) + "LAST"
	}
	b, _ := json.Marshal(p)
	return &api.MemoryReply{Data: b}, nil
}
func (c *memoryClient) List(_ context.Context, _ *api.Empty, _ ...grpc.CallOption) (*api.SessionList, error) {
	return &api.SessionList{Sessions: []*api.Session{
		{Id: "s", Agent: "claude", State: "stopped"},
		{Id: "active", Agent: "claude", State: "working"},
		{Id: "codex", Agent: "codex", State: "stopped"},
		{Id: "target", Agent: "claude", Account: "other", ProjectName: "renamed", State: "stopped"},
	}}, nil
}
func (c *memoryClient) CopyMemory(_ context.Context, r *api.CopyMemoryRequest, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.copies = append(c.copies, r)
	return &api.Receipt{Status: "Copied"}, nil
}
func TestMemoryBrowseStoppedSessionAndReturn(t *testing.T) {
	m := conversationModel()
	c := &memoryClient{}
	m.client = c
	m.current().State = "stopped"
	m.input.SetValue("draft")
	m.input.Focus()
	m.Update(m.openMemory(m.current())())
	if m.input.Focused() || !m.memoryPage.page.Directory {
		t.Fatal("browser not focused")
	}
	m.Update(m.memoryKey(tea.KeyMsg{Type: tea.KeyEnter})())
	m.Update(m.memoryKey(tea.KeyMsg{Type: tea.KeyEnter})())
	if c.reads[2].Path != "projects/MEMORY.md" {
		t.Fatal(c.reads)
	}
	for _, width := range []int{40, 80, 180} {
		m.terminalWidth = width
		m.height = 24
		m.memoryKey(tea.KeyMsg{Type: tea.KeyEnd})
		text := m.View()
		if !strings.Contains(ansi.Strip(text), "LAST") {
			t.Fatal("preview not scrollable")
		}
		for _, line := range strings.Split(text, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatal("overflow", line)
			}
		}
	}
	old := m.memoryPage
	m.memoryKey(tea.KeyMsg{Type: tea.KeyEsc})
	m.receiveMemory(memoryResult{page: old, request: old.request})
	if m.memoryPage != nil || m.input.Value() != "draft" || !m.input.Focused() {
		t.Fatal("browser did not restore composer")
	}
}
func TestMemoryCopyReviewAndDestination(t *testing.T) {
	m := conversationModel()
	c := &memoryClient{}
	m.client = c
	m.Update(m.openMemory(m.current())())
	m.Update(m.memoryKey(tea.KeyMsg{Type: tea.KeyEnter})())
	m.Update(m.startMemoryCopy()())
	copy := m.memoryPage.copy
	if len(copy.targets) != 1 || copy.targets[0].Id != "target" {
		t.Fatal("invalid target selection", copy.targets)
	}
	m.memoryCopyKey(tea.KeyMsg{Type: tea.KeyEnter})
	copy.input.SetValue("projects/renamed/memory/MEMORY.md")
	m.memoryCopyKey(tea.KeyMsg{Type: tea.KeyEnter})
	if copy.stage != "confirm" || copy.confirm {
		t.Fatal("copy confirmation not cancel-first")
	}
	m.memoryCopyKey(tea.KeyMsg{Type: tea.KeyEnter, Paste: true})
	if len(c.copies) != 0 || copy.stage != "confirm" {
		t.Fatal("paste triggered copy")
	}
	m.memoryCopyKey(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(m.memoryCopyKey(tea.KeyMsg{Type: tea.KeyEnter})())
	if len(c.copies) != 1 || c.copies[0].SessionId != "s" || c.copies[0].TargetId != "target" || c.copies[0].Path != "projects/MEMORY.md" || c.copies[0].TargetPath != "projects/renamed/memory/MEMORY.md" {
		t.Fatal(c.copies)
	}
	if m.memoryPage.copy != nil || m.memoryPage.message != "Copied" {
		t.Fatal("completion missing")
	}
}
func TestMemoryProjectShortcutDoesNotResume(t *testing.T) {
	m := conversationModel()
	c := &memoryClient{}
	m.client = c
	s := m.current()
	s.State = "stopped"
	m.panelProjects = []*api.Project{{Id: s.ProjectId, Name: "renamed"}}
	m.allSessions = []*api.Session{s}
	m.panelIndex = 1
	m.panelFocus = true
	m.projectView = true
	cmd := m.panelKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if cmd == nil || m.memoryPage == nil || !m.projectView {
		t.Fatal("project shortcut failed")
	}
	m.Update(cmd())
	if c.reads[0].SessionId != s.Id {
		t.Fatal("wrong session")
	}
	m.memoryKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.projectView || !m.panelFocus {
		t.Fatal("project focus lost")
	}
}

func TestMemoryCopyMouseAndCancel(t *testing.T) {
	m := conversationModel()
	client := &memoryClient{}
	m.client = client
	m.Update(m.openMemory(m.current())())
	for _, confirm := range []bool{false, true} {
		m.Update(m.startMemoryCopy()())
		c := m.memoryPage.copy
		m.View()
		row := -1
		for y, action := range c.hits {
			if action == 0 {
				row = y - c.viewOffset
			}
		}
		if row < 0 {
			t.Fatal("target not clickable")
		}
		m.memoryMouse(tea.MouseMsg{X: 3, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if c.stage != "path" {
			t.Fatal("target click failed")
		}
		m.memoryCopyKey(tea.KeyMsg{Type: tea.KeyEnter})
		m.View()
		wanted := -1
		if confirm {
			wanted = -2
		}
		row = -1
		for y, action := range c.hits {
			if action == wanted {
				row = y - c.viewOffset
			}
		}
		if row < 0 {
			t.Fatal("confirmation not clickable")
		}
		cmd := m.memoryMouse(tea.MouseMsg{X: 3, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if !confirm {
			if cmd != nil || len(client.copies) != 0 || m.memoryPage.copy != nil {
				t.Fatal("cancel copied data")
			}
		} else {
			if cmd == nil {
				t.Fatal("copy click failed")
			}
			m.Update(cmd())
			if len(client.copies) != 1 {
				t.Fatal(client.copies)
			}
		}
	}
}
