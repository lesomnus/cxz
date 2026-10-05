package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/sessionalias"
)

type renameResult struct {
	id, alias string
	project   bool
	err       error
}

func (m *model) startRename() tea.Cmd {
	return m.renameSession(m.current())
}

func (m *model) renameSession(s *api.Session) tea.Cmd {
	if s == nil {
		return nil
	}
	m.aliasInput = textinput.New()
	m.aliasInput.Cursor.Style = inputCursorStyle
	m.aliasInput.Prompt = ""
	m.aliasInput.CharLimit = sessionalias.MaxLen
	// Match the room the session row gives a name, so a long alias scrolls in the
	// field instead of being cut off by the renderer.
	m.aliasInput.Width = max(3, min(sessionalias.MaxLen, m.panelScreenWidth()-17))
	m.aliasInput.SetValue(s.Alias)
	m.aliasInput.CursorEnd()
	m.textSelection = nil
	m.input.Blur()
	m.renaming = true
	m.renameID = s.Id
	m.renameProject = false
	m.notice = "Rename alias · Enter save · Esc cancel"
	return m.aliasInput.Focus()
}

func (m *model) renameKey(k tea.KeyMsg) tea.Cmd {
	if k.String() == "ctrl+d" {
		return tea.Quit
	}
	if m.renameBusy {
		return nil
	}
	switch k.String() {
	case "esc":
		m.renaming = false
		m.aliasInput.Blur()
		m.notice = "rename canceled"
		return nil
	case "enter":
		alias := m.aliasInput.Value()
		if m.renameProject {
			return m.saveProjectTitle(strings.TrimSpace(alias))
		}
		if !sessionalias.Valid(alias) {
			m.notice = sessionalias.Rule
			return nil
		}
		id := m.renameID
		m.renameBusy = true
		return func() tea.Msg {
			c, ok := m.client.(interface {
				RenameSession(context.Context, string, string) (*api.Session, error)
			})
			if !ok {
				return renameResult{id: id, err: fmt.Errorf("session rename unsupported")}
			}
			ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
			defer cancel()
			s, err := c.RenameSession(ctx, id, alias)
			if err != nil {
				return renameResult{id: id, err: err}
			}
			return renameResult{id: id, alias: s.Alias}
		}
	}
	var cmd tea.Cmd
	m.aliasInput, cmd = m.aliasInput.Update(k)
	return cmd
}

func (m *model) startProjectRename() tea.Cmd {
	if m.busy || m.creating {
		return nil
	}
	p := m.project
	rows := m.panelRows()
	if m.panelFocus || m.projectView {
		if m.panelIndex >= 0 && m.panelIndex < len(rows) {
			p = rows[m.panelIndex].project
		}
	} else if current := m.current(); current != nil {
		for _, row := range rows {
			if row.project.Id == current.ProjectId {
				p = row.project
				break
			}
		}
	}
	if p == nil || p.State == "connection" {
		m.notice = "Select a project to rename."
		return nil
	}
	for i, row := range rows {
		if row.session == nil && row.project.Id == p.Id {
			m.panelIndex = i
			break
		}
	}
	m.panelFocus = true
	m.aliasInput = textinput.New()
	m.aliasInput.Cursor.Style = inputCursorStyle
	m.aliasInput.Prompt = ""
	m.aliasInput.CharLimit = 200
	m.aliasInput.Width = max(3, m.panelScreenWidth()-4)
	m.aliasInput.SetValue(strings.TrimSuffix(p.Name, m.connectionLabel(p.Id)))
	m.aliasInput.CursorEnd()
	m.textSelection = nil
	m.input.Blur()
	m.renaming, m.renameProject, m.renameID = true, true, p.Id
	m.notice = "Project title · Enter save · Esc cancel"
	return m.aliasInput.Focus()
}
func (m *model) saveProjectTitle(title string) tea.Cmd {
	if title == "" || len(title) > 200 || strings.ContainsAny(title, "\r\n\t\x00") {
		m.notice = "Project title must be nonempty, at most 200 bytes, with no control whitespace"
		return nil
	}
	id := m.renameID
	m.renameBusy = true
	return func() tea.Msg {
		c, ok := m.client.(interface {
			RenameProject(context.Context, string, string) error
		})
		if !ok {
			return renameResult{id: id, project: true, err: fmt.Errorf("project rename unsupported")}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		return renameResult{id: id, alias: title, project: true, err: c.RenameProject(ctx, id, title)}
	}
}
