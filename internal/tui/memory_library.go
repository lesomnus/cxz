package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/memorylib"
)

type libraryPage struct {
	session          *api.Session
	request          memorylib.Request
	data             memorylib.Reply
	selected, offset int
	loading          bool
	message          string
	cancel           context.CancelFunc
	generation       uint64
	checked          map[string]bool
	dialog           string
	input            textinput.Model
	confirmed        memorylib.Request
	refreshed        time.Time
}
type libraryResult struct {
	page       *libraryPage
	generation uint64
	request    memorylib.Request
	data       memorylib.Reply
	err        error
}

func (m *model) openMemory(s *api.Session) tea.Cmd {
	if _, ok := m.client.(memorylib.Client); !ok {
		return m.openAgentMemory(s)
	}
	if s == nil {
		return nil
	}
	m.library = &libraryPage{session: &api.Session{Id: s.Id, ProjectId: s.ProjectId, Agent: s.Agent, Account: s.Account, Title: s.Title}, checked: map[string]bool{}, input: textinput.New()}
	m.library.input.CharLimit = 256
	m.input.Blur()
	m.clearPathHints()
	return m.loadLibrary(memorylib.Request{Action: "list"})
}
func (m *model) loadLibrary(q memorylib.Request) tea.Cmd {
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
	client := m.client.(memorylib.Client)
	id := p.session.Id
	return func() tea.Msg {
		defer cancel()
		data, err := client.Library(ctx, id, q)
		return libraryResult{p, gen, q, data, err}
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
	switch v.request.Action {
	case "list", "search", "read", "deleted_documents":
		if p.request.ID != v.request.ID || p.request.Document != v.request.Document || p.request.Action != v.request.Action || p.request.Trash != v.request.Trash {
			p.selected = 0
			p.offset = 0
		}
		p.request = v.request
		p.data = v.data
		p.selected = min(p.selected, max(0, m.libraryCount()-1))
		return nil
	default:
		q := p.request
		if v.request.Action == "delete" || v.request.Action == "restore" || v.request.Action == "purge" {
			if v.request.Document == "" {
				q = memorylib.Request{Action: "list", Trash: p.request.Trash}
			} else {
				q.Document = ""
			}
		}
		if v.request.Action == "snapshot" || v.request.Action == "merge" {
			q = memorylib.Request{Action: "list"}
			p.checked = map[string]bool{}
		}
		return m.loadLibrary(q)
	}
}
func (m *model) libraryCount() int {
	p := m.library
	if p.request.ID == "" {
		return len(p.data.Memories)
	}
	return len(p.data.Documents)
}
func (m *model) librarySelection() (string, string, string) {
	p := m.library
	if p.request.ID == "" {
		if len(p.data.Memories) > p.selected {
			return p.data.Memories[p.selected].ID, "", ""
		}
		return "", "", ""
	}
	if p.request.Document != "" {
		return p.request.ID, p.request.Document, p.data.Revision
	}
	if len(p.data.Documents) > p.selected {
		d := p.data.Documents[p.selected]
		return p.request.ID, d.Name, d.Revision
	}
	return p.request.ID, "", ""
}
func (m *model) libraryKey(k tea.KeyMsg) tea.Cmd {
	p := m.library
	if k.String() == "ctrl+d" && !k.Paste {
		if p.cancel != nil {
			p.cancel()
		}
		return tea.Quit
	}
	if p.dialog != "" {
		if k.String() == "esc" && !k.Paste {
			p.dialog = ""
			return nil
		}
		if p.dialog == "delete" {
			if k.String() == "y" && !k.Paste {
				p.dialog = ""
				return m.loadLibrary(p.confirmed)
			}
			return nil
		}
		if k.String() == "enter" && !k.Paste {
			action := p.dialog
			p.dialog = ""
			value := strings.TrimSpace(p.input.Value())
			if value == "" {
				return nil
			}
			q := p.confirmed
			q.Action = action
			q.Name = value
			if action == "search" {
				q = memorylib.Request{Action: "search", Query: value}
			}
			return m.loadLibrary(q)
		}
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(k)
		return cmd
	}
	if k.Paste {
		return nil
	}
	if p.loading && k.String() != "esc" {
		return nil
	}
	id, doc, rev := m.librarySelection()
	switch k.String() {
	case "esc":
		if p.cancel != nil {
			p.cancel()
		}
		m.library = nil
		return m.focusComposer()
	case "left", "backspace":
		if p.request.Document != "" {
			return m.loadLibrary(memorylib.Request{Action: "read", ID: p.request.ID, Trash: p.request.Trash})
		}
		return m.loadLibrary(memorylib.Request{Action: "list", Trash: p.request.Trash})
	case "enter", "right":
		if id == "" || p.request.Action == "deleted_documents" {
			return nil
		}
		return m.loadLibrary(memorylib.Request{Action: "read", ID: id, Document: doc, Trash: p.request.Trash})
	case "o":
		s := p.session
		m.library = nil
		return m.openAgentMemory(s)
	case "r":
		return m.loadLibrary(p.request)
	case "t":
		if p.request.ID != "" && !p.request.Trash {
			if p.request.Action == "deleted_documents" {
				return m.loadLibrary(memorylib.Request{Action: "read", ID: p.request.ID})
			}
			return m.loadLibrary(memorylib.Request{Action: "deleted_documents", ID: p.request.ID})
		}
		return m.loadLibrary(memorylib.Request{Action: "list", Trash: !p.request.Trash})
	case "u":
		if p.request.Trash || p.request.Action == "deleted_documents" {
			return m.loadLibrary(memorylib.Request{Action: "restore", ID: id, Document: doc})
		}
	case "d", "delete":
		if id != "" && !p.request.Trash && p.request.Action != "deleted_documents" {
			p.dialog = "delete"
			p.confirmed = memorylib.Request{Action: "delete", ID: id, Document: doc, Revision: rev}
			p.message = "Move selected memory/document to trash? y confirms · Esc cancels"
		}
	case "x":
		if id != "" && (p.request.Trash || p.request.Action == "deleted_documents") {
			p.dialog = "delete"
			p.confirmed = memorylib.Request{Action: "purge", ID: id, Document: doc, Trash: p.request.Trash}
			p.message = "Permanently delete selected item? Cannot undo. y confirms · Esc cancels"
		}
	case "i":
		return m.loadLibrary(memorylib.Request{Action: "import"})
	case "s":
		p.dialog = "snapshot"
		p.confirmed = memorylib.Request{}
		p.input.SetValue("Memory " + time.Now().Format("2006-01-02 15:04"))
		return p.input.Focus()
	case "c":
		if id != "" && !p.request.Trash {
			p.dialog = "snapshot"
			p.confirmed = memorylib.Request{ID: id}
			p.input.SetValue("Saved memory")
			return p.input.Focus()
		}
	case "e":
		if id != "" {
			p.dialog = "rename"
			p.confirmed = memorylib.Request{ID: id, Trash: p.request.Trash}
			p.input.SetValue("")
			return p.input.Focus()
		}
	case "/":
		p.dialog = "search"
		p.input.SetValue("")
		return p.input.Focus()
	case " ":
		if p.request.ID == "" && id != "" && !p.request.Trash {
			p.checked[id] = !p.checked[id]
		}
	case "m":
		ids := []string{}
		for id, checked := range p.checked {
			if checked {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		if len(ids) < 2 {
			p.message = "Select at least two memories with Space"
			return nil
		}
		p.dialog = "merge"
		p.confirmed = memorylib.Request{IDs: ids}
		p.input.SetValue("Combined memory")
		return p.input.Focus()
	case "n":
		if id == "" || p.request.Trash {
			return nil
		}
		return m.startMemorySession(id)
	case "up", "down", "pgup", "pgdown", "home", "end":
		step := 1
		if k.String() == "pgup" || k.String() == "pgdown" {
			step = max(1, m.height-8)
		}
		if k.String() == "up" || k.String() == "pgup" {
			step = -step
		}
		if p.request.Document != "" {
			p.offset = max(0, p.offset+step)
			if k.String() == "home" {
				p.offset = 0
			}
			if k.String() == "end" {
				p.offset = 1 << 20
			}
		} else {
			p.selected = max(0, min(m.libraryCount()-1, p.selected+step))
			if k.String() == "home" {
				p.selected = 0
			}
			if k.String() == "end" {
				p.selected = max(0, m.libraryCount()-1)
			}
		}
	}
	return nil
}
func (m *model) libraryScreen() string {
	p := m.library
	width := m.settingsWidth()
	inner := max(1, width-4)
	title := "Project memory"
	if p.request.Trash || p.request.Action == "deleted_documents" {
		title += " · Trash"
	}
	if p.data.Memory != nil {
		title += " · " + p.data.Memory.Name
	}
	rows := []string{accent.Bold(true).Render(safeText(title)), clip(safeText(p.data.Location), inner), ""}
	body := []string{}
	if p.loading && p.data.Location == "" {
		body = append(body, "Loading…")
	} else if p.request.Document != "" {
		body = strings.Split(ansi.Hardwrap(safeText(p.data.Content), inner, true), "\n")
	} else if p.request.ID == "" {
		for i, item := range p.data.Memories {
			mark := "  "
			if p.checked[item.ID] {
				mark = "✓ "
			}
			kind := "session"
			if item.Snapshot {
				kind = "saved"
			}
			line := fmt.Sprintf("%s%s · %s · %s · %s", mark, item.Name, kind, item.Agent, attachmentSize(item.Size))
			if i == p.selected {
				line = accent.Render("❯ " + safeText(line))
			} else {
				line = "  " + safeText(line)
			}
			body = append(body, line)
		}
	} else {
		for i, d := range p.data.Documents {
			line := safeText(d.Name) + " · " + attachmentSize(d.Size)
			if i == p.selected {
				line = accent.Render("❯ " + line)
			} else {
				line = "  " + line
			}
			body = append(body, line)
		}
	}
	if len(body) == 0 {
		body = []string{"No memory yet. Agents can save notes through MCP; i imports native Markdown."}
	}
	footer := []string{clip(safeText(p.message), inner), "↑↓ Enter browse · / search · s save current · c copy selected · n new session", "Space select · m combine · e rename · d delete · t trash · u restore · x purge", "i import native · o original files · r refresh · ← parent · Esc close"}
	if p.dialog != "" && p.dialog != "delete" {
		footer[0] = safeText(p.dialog) + ": " + p.input.View()
		footer[1] = "Enter apply · Esc cancel"
	}
	capacity := max(1, m.height-len(rows)-len(footer))
	if p.request.Document == "" {
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
	for _, line := range footer {
		rows = append(rows, clip(line, inner))
	}
	return screen(strings.Join(rows, "\n"), width, m.height)
}
func (m *model) libraryMouse(v tea.MouseMsg) tea.Cmd {
	p := m.library
	if p.dialog != "" {
		return nil
	}
	if v.Button == tea.MouseButtonWheelUp {
		return m.libraryKey(tea.KeyMsg{Type: tea.KeyUp})
	}
	if v.Button == tea.MouseButtonWheelDown {
		return m.libraryKey(tea.KeyMsg{Type: tea.KeyDown})
	}
	if v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress && v.Y >= 3 && v.Y < m.height-4 && p.request.Document == "" {
		index := v.Y - 3 + p.offset
		if index >= 0 && index < m.libraryCount() {
			p.selected = index
			return m.libraryKey(tea.KeyMsg{Type: tea.KeyEnter})
		}
	}
	return nil
}
func (m *model) pollLibrary() tea.Cmd {
	p := m.library
	if p == nil || p.loading || p.dialog != "" || time.Since(p.refreshed) < 5*time.Second {
		return nil
	}
	return m.loadLibrary(p.request)
}
func (m *model) startMemorySession(id string) tea.Cmd {
	p := m.library
	var project *api.Project
	for _, candidate := range append([]*api.Project{m.project}, m.panelProjects...) {
		if candidate != nil && candidate.Id == p.session.ProjectId {
			project = candidate
			break
		}
	}
	if project == nil {
		p.message = "Project metadata unavailable; refresh the project first"
		return nil
	}
	// Freeze live memory before account selection; the source may keep changing.
	client := m.client.(memorylib.Client)
	ctx, cancel := context.WithTimeout(m.contextFor(p.session.Id), 30*time.Second)
	p.cancel = cancel
	session := p.session.Id
	p.loading = true
	p.generation++
	gen := p.generation
	return func() tea.Msg {
		defer cancel()
		data, e := client.Library(ctx, session, memorylib.Request{Action: "snapshot", ID: id, Name: "Session starting memory"})
		return memorySeed{p, gen, project, data, e}
	}
}

type memorySeed struct {
	page       *libraryPage
	generation uint64
	project    *api.Project
	data       memorylib.Reply
	err        error
}
