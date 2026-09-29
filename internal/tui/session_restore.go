package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

type sessionArchiveClient interface {
	ArchivedSessions(context.Context, string) ([]*api.Session, error)
	RestoreSession(context.Context, string) (*api.Session, error)
}

type sessionArchive struct {
	items         []*api.Session
	selected      int
	loading, busy bool
	message       string
}
type archiveLoaded struct {
	page  *sessionArchive
	items []*api.Session
	err   error
}
type archiveRestored struct {
	page    *sessionArchive
	session *api.Session
	err     error
}

func (m *model) openSessionArchive() tea.Cmd {
	if m.project == nil {
		return nil
	}
	p := &sessionArchive{loading: true}
	m.sessionArchive = p
	client, ok := m.client.(sessionArchiveClient)
	if !ok {
		p.loading = false
		p.message = "Update the host manager to restore deleted sessions."
		return nil
	}
	project, parent := m.project.Id, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 20*time.Second)
		defer cancel()
		items, err := client.ArchivedSessions(ctx, project)
		return archiveLoaded{p, ProjectSessions(items, nil), err}
	}
}

func (m *model) sessionArchiveKey(k tea.KeyMsg) tea.Cmd {
	p := m.sessionArchive
	if k.Paste {
		return nil
	}
	switch k.String() {
	case "esc", "ctrl+q":
		m.sessionArchive = nil
	case "up":
		p.selected = max(0, p.selected-1)
	case "down":
		p.selected = min(max(0, len(p.items)-1), p.selected+1)
	case "r":
		if !p.busy {
			return m.openSessionArchive()
		}
	case "enter":
		if p.busy || p.loading || len(p.items) == 0 {
			return nil
		}
		client, ok := m.client.(sessionArchiveClient)
		if !ok {
			return nil
		}
		p.busy = true
		id, parent := p.items[p.selected].Id, m.ctx
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(parent, 30*time.Second)
			defer cancel()
			v, err := client.RestoreSession(ctx, id)
			return archiveRestored{p, v, err}
		}
	}
	return nil
}

func (m *model) sessionArchiveScreen() string {
	p := m.sessionArchive
	rows := []string{"", strong.Render("Restore a deleted session"), "Conversation, agent thread, files and session settings are retained.", "Restored sessions receive an available alias; resume when ready.", ""}
	capacity := max(1, (m.height-10)/2)
	start := max(0, p.selected-capacity+1)
	for i := start; i < min(len(p.items), start+capacity); i++ {
		s := p.items[i]
		name := s.Title
		if name == "" {
			name = s.Id
		}
		label := "  " + pickerLabel(name)
		if i == p.selected {
			label = focus.Render("› " + pickerLabel(name))
		}
		rows = append(rows, clip(label, max(1, m.width-4)), muted.Render(fmt.Sprintf("  %s · %s · %s", providerLabel(s.Agent), pickerLabel(s.Account), time.UnixMilli(s.CreatedAt).Local().Format("2006-01-02 15:04"))))
	}
	if p.loading {
		rows = append(rows, "Loading deleted sessions…")
	} else if len(p.items) == 0 {
		rows = append(rows, "No deleted sessions in this project.")
	}
	if p.busy {
		rows = append(rows, "Restoring…")
	}
	rows = append(rows, "", warning.Render(safeText(p.message)), "↑/↓ select · Enter restore · r refresh · Esc back", "Permanent installation cleanup: cxz purge (interactive review).")
	return screen(indentBlock(strings.Join(rows, "\n")), m.width, m.height)
}
