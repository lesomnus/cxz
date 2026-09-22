package tui

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

type memoryCopy struct {
	hits             map[int]int
	source           string
	targets          []*api.Session
	selected, offset int
	viewOffset       int
	stage            string
	input            textinput.Model
	confirm          bool
	message          string
}
type memoryTargets struct {
	page     *memoryPage
	copy     *memoryCopy
	sessions []*api.Session
	err      error
}
type memoryCopied struct {
	page    *memoryPage
	copy    *memoryCopy
	message string
	err     error
}

func (m *model) startMemoryCopy() tea.Cmd {
	p := m.memoryPage
	if p.loading || p.err != "" {
		return nil
	}
	source := p.page.Path
	if p.page.Directory {
		if len(p.page.Entries) == 0 {
			return nil
		}
		source = path.Join(source, p.page.Entries[p.selected].Name)
	}
	if source == "" {
		return nil
	}
	c := &memoryCopy{source: source, stage: "loading", input: textinput.New()}
	c.input.CharLimit = 4096
	c.input.SetValue(source)
	p.copy = c
	client, ctx := m.client, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		out, err := client.List(ctx, &api.Empty{})
		result := memoryTargets{page: p, copy: c, err: err}
		if out != nil {
			result.sessions = out.Sessions
		}
		return result
	}
}
func (m *model) receiveMemoryTargets(r memoryTargets) {
	p := m.memoryPage
	if p == nil || p != r.page || p.copy != r.copy {
		return
	}
	c := p.copy
	c.stage = "target"
	if r.err != nil {
		c.message = r.err.Error()
		return
	}
	for _, s := range r.sessions {
		if s.Id == p.session.Id || s.Agent != p.session.Agent || m.connectionName(s.Id) != m.connectionName(p.session.Id) {
			continue
		}
		switch s.State {
		case "starting", "idle", "working", "waiting_input":
			continue
		}
		c.targets = append(c.targets, s)
	}
	if len(c.targets) == 0 {
		c.message = "No stopped target sessions for this agent. Create/initialize a target session, then stop it before copying."
	}
}
func (m *model) receiveMemoryCopied(r memoryCopied) {
	p := m.memoryPage
	if p == nil || p != r.page || p.copy != r.copy {
		return
	}
	if r.err != nil {
		p.copy.stage = "path"
		p.copy.message = r.err.Error()
		p.copy.input.Focus()
		return
	}
	p.message = r.message
	p.copy = nil
}
func (m *model) memoryCopyKey(k tea.KeyMsg) tea.Cmd {
	p := m.memoryPage
	c := p.copy
	if k.String() == "ctrl+d" && !k.Paste {
		return tea.Quit
	}
	if k.String() == "esc" && !k.Paste {
		p.copy = nil
		return nil
	}
	if !k.Paste {
		switch k.String() {
		case "pgup":
			c.viewOffset = max(0, c.viewOffset-max(1, m.height-2))
			return nil
		case "pgdown":
			c.viewOffset += max(1, m.height-2)
			return nil
		}
	}
	if c.stage == "path" {
		if k.String() == "enter" && !k.Paste {
			if strings.TrimSpace(c.input.Value()) == "" {
				c.message = "Enter a path relative to the target agent profile."
				return nil
			}
			c.viewOffset = 0
			c.stage = "confirm"
			c.confirm = false
			c.input.Blur()
			return nil
		}
		var cmd tea.Cmd
		c.input, cmd = c.input.Update(k)
		return cmd
	}
	if k.Paste {
		return nil
	}
	switch c.stage {
	case "target":
		switch k.String() {
		case "up":
			c.selected = max(0, c.selected-1)
		case "down":
			c.selected = min(max(0, len(c.targets)-1), c.selected+1)
		case "home":
			c.selected = 0
		case "end":
			c.selected = max(0, len(c.targets)-1)
		case "enter":
			if len(c.targets) > 0 {
				c.viewOffset = 0
				c.stage = "path"
				c.message = ""
				return c.input.Focus()
			}
		}
	case "confirm":
		switch k.String() {
		case "tab", "shift+tab", "left", "right":
			c.confirm = !c.confirm
		case "enter":
			if !c.confirm {
				p.copy = nil
				return nil
			}
			target := c.targets[c.selected]
			r := &api.CopyMemoryRequest{SessionId: p.session.Id, Path: c.source, TargetId: target.Id, TargetPath: c.input.Value()}
			c.stage = "copying"
			client, ctx := m.client, m.ctx
			return func() tea.Msg {
				ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
				defer cancel()
				out, err := client.CopyMemory(ctx, r)
				result := memoryCopied{page: p, copy: c, err: err}
				if out != nil {
					result.message = out.Status
				}
				return result
			}
		}
	}
	return nil
}
func memorySessionLabel(s *api.Session) string {
	name := s.Alias
	if name == "" {
		name = s.Id
	}
	project := s.ProjectName
	if project == "" {
		project = s.ProjectId
	}
	return fmt.Sprintf("%s · %s/%s · %s · %s", name, s.Agent, s.Account, project, s.State)
}
func (m *model) memoryCopyScreen() string {
	c := m.memoryPage.copy
	width := m.settingsWidth()
	inner := max(1, width-4)
	c.hits = map[int]int{}
	actions := map[int]int{}
	lines := []string{accent.Bold(true).Render("Copy retained agent data"), "From: " + safeText(memorySessionLabel(m.memoryPage.session)), "Path: " + safeText(c.source), ""}
	footer := "PgUp/PgDn scroll · Esc cancel"
	switch c.stage {
	case "loading":
		lines = append(lines, "Loading target sessions…")
	case "copying":
		lines = append(lines, "Copying… Closing this dialog does not cancel the copy.")
	case "target":
		lines = append(lines, "Choose a stopped target session:")
		capacity := max(1, m.height-len(lines)-4)
		c.offset = max(0, min(c.offset, c.selected))
		c.offset = max(c.offset, c.selected-capacity+1)
		for i := c.offset; i < min(len(c.targets), c.offset+capacity); i++ {
			label := "  " + safeText(memorySessionLabel(c.targets[i]))
			if i == c.selected {
				label = accent.Render("› " + safeText(memorySessionLabel(c.targets[i])))
			}
			actions[len(lines)] = i
			lines = append(lines, clip(label, inner))
		}
		footer = "↑↓ select · Enter choose · Esc cancel"
	case "path":
		lines = append(lines, "To: "+safeText(memorySessionLabel(c.targets[c.selected])), "Destination path (relative to agent profile):")
		c.input.Width = max(1, inner-2)
		lines = append(lines, c.input.View(), "", "Existing files are never overwritten.", "Enter to review · Esc cancel")
	case "confirm":
		lines = append(lines, "To: "+safeText(memorySessionLabel(c.targets[c.selected])), "Destination: "+safeText(c.input.Value()), "", "Copy without changing login or conversation identity?", "")
		cancel, confirm := "[ Cancel ]", "[ Copy ]"
		if c.confirm {
			confirm = accent.Render("› " + confirm)
		} else {
			cancel = accent.Render("› " + cancel)
		}
		actions[len(lines)] = -1
		lines = append(lines, cancel)
		actions[len(lines)] = -2
		lines = append(lines, confirm)
		footer = "←/→ choose · Enter select · Esc cancel"
	}
	if c.message != "" {
		lines = append(lines, "", safeText(c.message))
	}
	var wrapped []string
	for i, line := range lines {
		if action, ok := actions[i]; ok {
			c.hits[len(wrapped)] = action
		}
		wrapped = append(wrapped, strings.Split(ansi.Hardwrap(line, inner, true), "\n")...)
	}
	c.viewOffset = max(0, min(c.viewOffset, max(0, len(wrapped)-max(1, m.height-1))))
	wrapped = wrapped[c.viewOffset:min(len(wrapped), c.viewOffset+max(1, m.height-1))]
	rows := make([]string, max(1, m.height-1))
	for i, line := range wrapped {
		if i < len(rows) {
			rows[i] = "  " + line
		}
	}
	rows = append(rows, "  "+muted.Render(clip(footer, inner)))
	return screen(strings.Join(rows, "\n"), width, m.height)
}

func (m *model) memoryCopyMouse(v tea.MouseMsg) tea.Cmd {
	c := m.memoryPage.copy
	switch v.Button {
	case tea.MouseButtonWheelUp:
		if c.stage == "target" {
			return m.memoryCopyKey(tea.KeyMsg{Type: tea.KeyUp})
		}
		c.viewOffset = max(0, c.viewOffset-3)
	case tea.MouseButtonWheelDown:
		if c.stage == "target" {
			return m.memoryCopyKey(tea.KeyMsg{Type: tea.KeyDown})
		}
		c.viewOffset += 3
	case tea.MouseButtonLeft:
		if v.Action != tea.MouseActionPress || v.X < 2 || v.X >= m.settingsWidth()-2 || v.Y >= m.height-1 {
			return nil
		}
		action, ok := c.hits[v.Y+c.viewOffset]
		if !ok {
			return nil
		}
		if c.stage == "target" && action >= 0 && action < len(c.targets) {
			c.selected = action
			return m.memoryCopyKey(tea.KeyMsg{Type: tea.KeyEnter})
		}
		if c.stage == "confirm" && action < 0 {
			c.confirm = action == -2
			return m.memoryCopyKey(tea.KeyMsg{Type: tea.KeyEnter})
		}
	}
	return nil
}
