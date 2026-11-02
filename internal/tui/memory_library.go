package tui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/memoryclient"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/protobuf/proto"
	"strings"
	"time"
)

type libraryPage struct {
	session                *api.Session
	rows                   []*resource.Memory
	document               *resource.Memory
	selected, offset       int
	loading, trash         bool
	query, message, dialog string
	input                  textinput.Model
	cancel                 context.CancelFunc
	generation             uint64
	refreshed              time.Time
	confirmed              *resource.Memory
}
type libraryResult struct {
	page       *libraryPage
	generation uint64
	rows       []*resource.Memory
	document   *resource.Memory
	err        error
}

func (m *model) openMemory(s *api.Session) tea.Cmd {
	if s == nil {
		return nil
	}
	if _, ok := m.client.(memoryclient.Client); !ok {
		return m.openAgentMemory(s)
	}
	m.library = &libraryPage{session: proto.Clone(s).(*api.Session), input: textinput.New()}
	m.library.input.CharLimit = 256
	m.input.Blur()
	m.clearPathHints()
	return m.loadLibrary()
}
func (m *model) loadLibrary() tea.Cmd {
	p := m.library
	if p == nil {
		return nil
	}
	if p.cancel != nil {
		p.cancel()
	}
	ctx, cancel := context.WithTimeout(m.contextFor(p.session.Id), 30*time.Second)
	p.cancel = cancel
	p.generation++
	gen := p.generation
	p.loading = true
	p.message = ""
	p.refreshed = time.Now()
	return func() tea.Msg {
		defer cancel()
		client, project, err := m.client.(memoryclient.Client).MemoryResources(ctx, p.session.ProjectId)
		if err != nil {
			return libraryResult{page: p, generation: gen, err: err}
		}
		var rows []*resource.Memory
		after := ""
		scope := resource.MemoryScope_MEMORY_SCOPE_ACTIVE
		if p.trash {
			scope = resource.MemoryScope_MEMORY_SCOPE_TRASH
		}
		for {
			page, e := client.List(ctx, resource.MemoryListRequest_builder{Project: projectid.Ref(project), Scope: scope, Query: p.query, After: after, Size: 500}.Build())
			if e != nil {
				return libraryResult{page: p, generation: gen, err: e}
			}
			rows = append(rows, page.GetItems()...)
			after = page.GetNext()
			if after == "" {
				break
			}
		}
		return libraryResult{page: p, generation: gen, rows: rows}
	}
}
func (m *model) receiveLibrary(v libraryResult) tea.Cmd {
	p := m.library
	if p == nil || p != v.page || p.generation != v.generation {
		return nil
	}
	p.loading = false
	if v.err != nil {
		p.message = v.err.Error()
		return nil
	}
	if v.document != nil {
		p.document = v.document
		p.offset = 0
	} else {
		p.rows = v.rows
		p.document = nil
		p.selected = min(p.selected, max(0, len(p.rows)-1))
		p.offset = 0
	}
	return nil
}
func (m *model) selectedMemory() *resource.Memory {
	p := m.library
	if p.document != nil {
		return p.document
	}
	if p.selected >= 0 && p.selected < len(p.rows) {
		return p.rows[p.selected]
	}
	return nil
}
func (m *model) memoryAction(action string, selected *resource.Memory) tea.Cmd {
	p := m.library
	if p == nil || selected == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(m.contextFor(p.session.Id), 30*time.Second)
	p.cancel = cancel
	p.generation++
	gen := p.generation
	p.loading = true
	name := p.input.Value()
	return func() tea.Msg {
		defer cancel()
		client, project, err := m.client.(memoryclient.Client).MemoryResources(ctx, p.session.ProjectId)
		if err != nil {
			return libraryResult{page: p, generation: gen, err: err}
		}
		ref := resource.MemoryRef_builder{Id: selected.GetId(), Project: projectid.Ref(project)}.Build()
		var doc *resource.Memory
		switch action {
		case "read":
			doc, err = client.Get(ctx, resource.MemoryGetRequest_builder{Ref: ref, Select: resource.MemorySelect_builder{All: proto.Bool(true), Project: resource.ProjectSelect_builder{All: proto.Bool(true)}.Build()}.Build()}.Build())
		case "rename":
			doc, err = client.Patch(ctx, resource.MemoryPatchRequest_builder{Ref: ref, DateUpdated: selected.GetDateUpdated(), Name: proto.String(name)}.Build())
		case "delete":
			_, err = client.Erase(ctx, ref)
		case "restore":
			_, err = client.Restore(ctx, resource.MemoryRestoreRequest_builder{Ref: ref, DateUpdated: selected.GetDateUpdated()}.Build())
		case "purge":
			_, err = client.Purge(ctx, resource.MemoryPurgeRequest_builder{Ref: ref, DateUpdated: selected.GetDateUpdated()}.Build())
		case "copy":
			source, e := client.Get(ctx, resource.MemoryGetRequest_builder{Ref: ref, Select: resource.MemorySelect_builder{All: proto.Bool(true)}.Build()}.Build())
			err = e
			if err == nil {
				_, err = client.Add(ctx, resource.MemoryAddRequest_builder{Project: projectid.Ref(project), Name: name, Content: source.GetContent(), Labels: source.GetLabels(), OwnerSession: localMemoryOwner(p.session.Id)}.Build())
			}
		}
		if action == "read" || action == "rename" {
			return libraryResult{page: p, generation: gen, document: doc, err: err}
		}
		return libraryActionResult{page: p, generation: gen, err: err}
	}
}

type libraryActionResult struct {
	page       *libraryPage
	generation uint64
	err        error
}

func (m *model) libraryKey(key tea.KeyMsg) tea.Cmd {
	p := m.library
	if p == nil {
		return nil
	}
	text := key.String()
	if p.dialog != "" {
		if text == "esc" {
			p.dialog = ""
			return nil
		}
		if p.dialog == "delete" || p.dialog == "purge" {
			if text == "y" {
				action, selected := p.dialog, p.confirmed
				p.dialog = ""
				return m.memoryAction(action, selected)
			}
			return nil
		}
		if text == "enter" {
			action := p.dialog
			p.dialog = ""
			if action == "search" {
				p.query = p.input.Value()
				p.selected = 0
				return m.loadLibrary()
			}
			return m.memoryAction(action, p.confirmed)
		}
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(key)
		return cmd
	}
	switch text {
	case "esc":
		if p.document != nil {
			p.document = nil
			p.offset = 0
			return nil
		}
		if p.cancel != nil {
			p.cancel()
		}
		m.library = nil
		return nil
	case "left", "h":
		p.document = nil
		p.offset = 0
	case "up", "k":
		if p.document != nil {
			p.offset = max(0, p.offset-1)
		} else {
			p.selected = max(0, p.selected-1)
		}
	case "down", "j":
		if p.document != nil {
			p.offset++
		} else {
			p.selected = min(max(0, len(p.rows)-1), p.selected+1)
		}
	case "enter", "right", "l":
		return m.memoryAction("read", m.selectedMemory())
	case "r":
		return m.loadLibrary()
	case "t":
		p.trash = !p.trash
		p.selected = 0
		return m.loadLibrary()
	case "u":
		if p.trash {
			return m.memoryAction("restore", m.selectedMemory())
		}
	case "d":
		if !p.trash && m.selectedMemory() != nil {
			p.dialog = "delete"
			p.confirmed = m.selectedMemory()
			p.message = "Move this memory to trash? y confirms; Esc cancels."
		}
	case "x":
		if p.trash && m.selectedMemory() != nil {
			p.dialog = "purge"
			p.confirmed = m.selectedMemory()
			p.message = "Permanently delete this memory and its versions? y confirms."
		}
	case "/":
		p.dialog = "search"
		p.input.SetValue(p.query)
		return p.input.Focus()
	case "e", "c":
		if m.selectedMemory() != nil && !p.trash {
			p.dialog = "rename"
			if text == "c" {
				p.dialog = "copy"
			}
			p.confirmed = m.selectedMemory()
			p.input.SetValue(p.confirmed.GetName())
			return p.input.Focus()
		}
	case "n":
		if !p.trash {
			return m.startMemorySession()
		}
	case "o":
		s := p.session
		m.library = nil
		return m.openAgentMemory(s)
	}
	return nil
}
func (m *model) libraryScreen() string {
	p := m.library
	inner := max(20, m.width-4)
	title := "Project memory"
	if p.trash {
		title += " · Trash"
	}
	if p.document != nil {
		title += " · " + p.document.GetName()
	}
	rows := []string{accent.Bold(true).Render(safeText(title)), ""}
	var body []string
	if p.document != nil {
		body = strings.Split(ansi.Hardwrap(safeText(p.document.GetContent()), inner, true), "\n")
	} else {
		for i, v := range p.rows {
			line := fmt.Sprintf("%s · revision %d · %s", v.GetName(), v.GetRevision(), v.GetOwnerSession())
			if i == p.selected {
				line = accent.Render("❯ " + safeText(line))
			} else {
				line = "  " + safeText(line)
			}
			body = append(body, line)
		}
	}
	if len(body) == 0 {
		if p.loading {
			body = []string{"Loading…"}
		} else {
			body = []string{"No memories match. Agents publish documents with memory_add."}
		}
	}
	footer := []string{clip(safeText(p.message), inner), "↑↓ Enter read · / search · e rename · c copy · r refresh", "d delete · t trash · u restore · x purge · o original files · Esc close"}
	if p.dialog != "" && p.dialog != "delete" && p.dialog != "purge" {
		footer[0] = safeText(p.dialog) + ": " + p.input.View()
	}
	capacity := max(1, m.height-len(rows)-len(footer))
	if p.document == nil {
		p.offset = max(0, min(p.offset, p.selected))
		p.offset = max(p.offset, p.selected-capacity+1)
	}
	p.offset = max(0, min(p.offset, max(0, len(body)-capacity)))
	for i := 0; i < capacity; i++ {
		line := ""
		if p.offset+i < len(body) {
			line = body[p.offset+i]
		}
		rows = append(rows, clip(line, inner))
	}
	rows = append(rows, footer...)
	return strings.Join(rows, "\n")
}
func (m *model) libraryMouse(v tea.MouseMsg) tea.Cmd {
	if v.Button == tea.MouseButtonWheelUp {
		return m.libraryKey(tea.KeyMsg{Type: tea.KeyUp})
	}
	if v.Button == tea.MouseButtonWheelDown {
		return m.libraryKey(tea.KeyMsg{Type: tea.KeyDown})
	}
	if v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress && v.Y >= 2 && v.Y < m.height-3 && m.library.document == nil {
		index := v.Y - 2 + m.library.offset
		if index >= 0 && index < len(m.library.rows) {
			m.library.selected = index
			return m.libraryKey(tea.KeyMsg{Type: tea.KeyEnter})
		}
	}
	return nil
}
func (m *model) pollLibrary() tea.Cmd {
	if m.library == nil || m.library.loading || m.library.dialog != "" || m.library.document != nil || time.Since(m.library.refreshed) < 5*time.Second {
		return nil
	}
	return m.loadLibrary()
}

func localMemoryOwner(id string) string {
	if _, local, ok := strings.Cut(id, "::"); ok {
		return local
	}
	return id
}

type memorySeed struct {
	page       *libraryPage
	generation uint64
	project    *api.Project
	data       *resource.Memory
	err        error
}

func (m *model) startMemorySession() tea.Cmd {
	p := m.library
	selected := m.selectedMemory()
	if selected == nil {
		return nil
	}
	var project *api.Project
	for _, candidate := range append([]*api.Project{m.project}, m.panelProjects...) {
		if candidate != nil && candidate.Id == p.session.ProjectId {
			project = candidate
			break
		}
	}
	if project == nil {
		p.message = "Project metadata unavailable; refresh first"
		return nil
	}
	ctx, cancel := context.WithTimeout(m.contextFor(p.session.Id), 30*time.Second)
	p.cancel = cancel
	p.generation++
	p.loading = true
	gen := p.generation
	return func() tea.Msg {
		defer cancel()
		client, local, err := m.client.(memoryclient.Client).MemoryResources(ctx, p.session.ProjectId)
		var data *resource.Memory
		if err == nil {
			data, err = client.Get(ctx, resource.MemoryGetRequest_builder{Ref: resource.MemoryRef_builder{Id: selected.GetId(), Project: projectid.Ref(local)}.Build(), Select: resource.MemorySelect_builder{All: proto.Bool(true)}.Build()}.Build())
		}
		return memorySeed{p, gen, project, data, err}
	}
}
