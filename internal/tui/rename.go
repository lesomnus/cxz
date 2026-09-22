package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/sessionalias"
)

type renameResult struct {
	id, alias string
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
	m.aliasInput.CharLimit = 7
	m.aliasInput.Width = 8
	m.aliasInput.SetValue(s.Alias)
	m.aliasInput.CursorEnd()
	m.textSelection = nil
	m.input.Blur()
	m.renaming = true
	m.renameID = s.Id
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
		if !sessionalias.Valid(alias) {
			m.notice = "Use 3–7 lowercase English letters"
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
