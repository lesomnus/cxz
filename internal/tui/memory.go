package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/memoryview"
)

type memoryPage struct {
	copy                  *memoryCopy
	message               string
	session               *api.Session
	page                  memoryview.Page
	selected, offset      int
	loading, inputFocused bool
	err                   string
	request               uint64
	cancel                context.CancelFunc
	preview               string
}
type memoryResult struct {
	page    *memoryPage
	request uint64
	data    memoryview.Page
	err     error
}

func (m *model) openMemory(s *api.Session) tea.Cmd {
	if s == nil {
		m.notice = "Select a session to inspect retained agent data."
		return nil
	}
	if m.memoryPage != nil && m.memoryPage.cancel != nil {
		m.memoryPage.cancel()
	}
	// Snapshot metadata: switching/refreshing sessions must not retarget this view.
	copy := &api.Session{Id: s.Id, Alias: s.Alias, Title: s.Title, Agent: s.Agent, Account: s.Account, ProjectName: s.ProjectName, ProjectId: s.ProjectId, State: s.State}
	m.memoryPage = &memoryPage{session: copy, inputFocused: m.input.Focused()}
	m.input.Blur()
	m.clearPathHints()
	return m.loadMemory("")
}
func (m *model) loadMemory(target string) tea.Cmd {
	p := m.memoryPage
	if p.cancel != nil {
		p.cancel()
	}
	ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
	p.cancel = cancel
	p.request++
	request := p.request
	p.loading = true
	p.err = ""
	p.offset = 0
	p.selected = 0
	p.preview = ""
	p.page = memoryview.Page{Path: target, Directory: true}
	client, id := m.client, p.session.Id
	return func() tea.Msg {
		defer cancel()
		out, err := client.Memory(ctx, &api.MemoryRequest{SessionId: id, Path: target})
		result := memoryResult{page: p, request: request, err: err}
		if err == nil {
			result.err = json.Unmarshal(out.Data, &result.data)
		}
		return result
	}
}
func (m *model) receiveMemory(r memoryResult) {
	p := m.memoryPage
	if p == nil || p != r.page || p.request != r.request {
		return
	}
	p.loading = false
	if r.err != nil {
		p.err = r.err.Error() + "\nIf Memory RPC is unavailable, update cxz and run cxz install --recreate on the host."
		return
	}
	p.page = r.data
	if !p.page.Directory {
		p.preview = highlightCode(p.page.Content, p.page.Path)
	}
}
func (m *model) memoryParent() tea.Cmd {
	p := m.memoryPage
	if p.page.Path == "" {
		return nil
	}
	parent := path.Dir(p.page.Path)
	if parent == "." {
		parent = ""
	}
	return m.loadMemory(parent)
}
func (m *model) memoryOpen() tea.Cmd {
	p := m.memoryPage
	if p.loading || p.err != "" || !p.page.Directory || len(p.page.Entries) == 0 {
		return nil
	}
	return m.loadMemory(path.Join(p.page.Path, p.page.Entries[p.selected].Name))
}
func (m *model) memoryKey(k tea.KeyMsg) tea.Cmd {
	p := m.memoryPage
	if p.copy != nil {
		return m.memoryCopyKey(k)
	}
	if k.Paste {
		return nil
	}
	switch k.String() {
	case "ctrl+d":
		if p.cancel != nil {
			p.cancel()
		}
		return tea.Quit
	case "esc":
		if p.cancel != nil {
			p.cancel()
		}
		m.memoryPage = nil
		if p.inputFocused {
			return m.input.Focus()
		}
	case "left", "backspace":
		return m.memoryParent()
	case "enter", "right":
		return m.memoryOpen()
	case "c":
		return m.startMemoryCopy()
	case "r":
		return m.loadMemory(p.page.Path)
	case "up", "down", "pgup", "pgdown", "home", "end":
		step := 1
		_, capacity := m.memoryLayout()
		if k.String() == "pgup" || k.String() == "pgdown" {
			step = capacity
		}
		if k.String() == "up" || k.String() == "pgup" {
			step = -step
		}
		if p.page.Directory && p.err == "" {
			p.selected = max(0, min(len(p.page.Entries)-1, p.selected+step))
			if k.String() == "home" {
				p.selected = 0
			}
			if k.String() == "end" {
				p.selected = max(0, len(p.page.Entries)-1)
			}
			p.offset = max(0, min(p.offset, p.selected))
			p.offset = max(p.offset, p.selected-capacity+1)
		} else {
			p.offset = max(0, p.offset+step)
			if k.String() == "home" {
				p.offset = 0
			}
			if k.String() == "end" {
				p.offset = 1 << 20
			}
		}
	}
	return nil
}
func (m *model) memoryLayout() ([]string, int) {
	p := m.memoryPage
	width := max(1, m.settingsWidth()-4)
	name := p.session.Alias
	if name == "" {
		name = p.session.Id
	}
	header := []string{accent.Bold(true).Render("Agent data · " + safeText(name)),
		safeText(fmt.Sprintf("%s/%s · %s · %s", p.session.Agent, p.session.Account, p.session.ProjectName, p.session.State))}
	location := p.page.Location
	if location == "" {
		location = "/" + p.page.Path
	}
	header = append(header, strings.Split(ansi.Hardwrap(safeText(location), width, true), "\n")...)
	header = append(header, muted.Render("Memory, instructions and native history · c copy"), "")
	if p.message != "" {
		header = append(header, strings.Split(ansi.Hardwrap(safeText(p.message), width, true), "\n")...)
	}
	if len(header) > max(1, m.height-3) {
		header = header[:max(1, m.height-3)]
	}
	return header, max(1, m.height-len(header)-1)
}
func (m *model) memoryMouse(v tea.MouseMsg) tea.Cmd {
	if m.memoryPage.copy != nil {
		return m.memoryCopyMouse(v)
	}
	switch v.Button {
	case tea.MouseButtonWheelUp:
		return m.memoryKey(tea.KeyMsg{Type: tea.KeyUp})
	case tea.MouseButtonWheelDown:
		return m.memoryKey(tea.KeyMsg{Type: tea.KeyDown})
	case tea.MouseButtonLeft:
		if v.Action != tea.MouseActionPress || v.X < 2 || v.X >= m.settingsWidth()-2 {
			return nil
		}
		p := m.memoryPage
		header, capacity := m.memoryLayout()
		row := v.Y - len(header)
		if row < 0 || row >= capacity || !p.page.Directory {
			return nil
		}
		index := p.offset + row
		if index >= 0 && index < len(p.page.Entries) {
			p.selected = index
			return m.memoryOpen()
		}
	}
	return nil
}
func (m *model) memoryScreen() string {
	p := m.memoryPage
	if p.copy != nil {
		return m.memoryCopyScreen()
	}
	width := m.settingsWidth()
	inner := max(1, width-4)
	header, capacity := m.memoryLayout()
	var body []string
	switch {
	case p.loading:
		body = []string{"Loading retained agent data…"}
	case p.err != "":
		body = strings.Split(ansi.Hardwrap(safeText(p.err), inner, true), "\n")
	case p.page.Directory:
		for i, entry := range p.page.Entries {
			name := safeText(entry.Name)
			if entry.Directory {
				name += "/"
			} else {
				name += fmt.Sprintf("  (%d B)", entry.Size)
			}
			if i == p.selected {
				name = accent.Render("› " + name)
			} else {
				name = "  " + name
			}
			body = append(body, name)
		}
		if p.page.Note != "" {
			body = append(body, muted.Render(p.page.Note))
		}
	default:
		if p.page.Note != "" {
			body = append(body, warning.Render(p.page.Note))
		}
		body = append(body, strings.Split(ansi.Hardwrap(p.preview, max(1, inner-2), true), "\n")...)
	}
	p.offset = max(0, min(p.offset, max(0, len(body)-capacity)))
	rows := make([]string, 0, m.height)
	for _, line := range header {
		rows = append(rows, "  "+clip(line, inner))
	}
	for i := 0; i < capacity; i++ {
		line := ""
		if p.offset+i < len(body) {
			line = clip(body[p.offset+i], max(1, inner-2))
		}
		rows = append(rows, "  "+panelBackground(" "+line+strings.Repeat(" ", max(0, inner-2-ansi.StringWidth(line)))+" "))
	}
	footer := "↑↓ select/scroll · Enter open · ← parent · c copy · r refresh · Esc close"
	rows = append(rows, "  "+muted.Render(clip(footer, inner)))
	return screen(strings.Join(rows, "\n"), width, m.height)
}
