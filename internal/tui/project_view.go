package tui

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

type ProjectCreator func(io.Reader, io.Writer, io.Writer) (*api.Session, error)

func ProjectSessions(sessions []*api.Session, p *api.Project) []*api.Session {
	var out []*api.Session
	for _, s := range sessions {
		if p == nil || s.ProjectId == p.Id {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt == out[j].CreatedAt {
			return out[i].Id > out[j].Id
		}
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out
}

func (m *model) backToProject() {
	if m.project == nil {
		if s := m.current(); s != nil {
			m.project = &api.Project{Id: s.ProjectId, Name: s.ProjectName, Alias: s.ProjectAlias, Workspace: s.Workspace}
		}
	}
	if m.project == nil {
		return
	}
	m.projectView = true
	m.sessions = ProjectSessions(m.sessions, m.project)
	m.selected = max(0, min(m.selected, len(m.sessions)-1))
	m.creating = false
	m.focusList = false
	m.deletingID = ""
	m.wantID = ""
	m.input.Reset()
	if m.watchCancel != nil {
		m.watchCancel()
		m.watchCancel = nil
	}
	m.watchID = ""
}

type createProjectExec struct {
	create   ProjectCreator
	in       io.Reader
	out, err io.Writer
	session  *api.Session
}

func (e *createProjectExec) SetStdin(v io.Reader)  { e.in = v }
func (e *createProjectExec) SetStdout(v io.Writer) { e.out = v }
func (e *createProjectExec) SetStderr(v io.Writer) { e.err = v }
func (e *createProjectExec) Run() error {
	s, err := e.create(e.in, e.out, e.err)
	e.session = s
	return err
}

func (m *model) projectKey(key tea.KeyMsg) tea.Cmd {
	if key.String() == "ctrl+c" {
		return tea.Quit
	}
	if m.busy {
		return nil
	}
	if m.deletingID != "" {
		if key.String() == "esc" || key.String() == "n" {
			m.deletingID = ""
			m.notice = "deletion canceled"
			return nil
		}
		if key.String() != "y" {
			return nil
		}
		id := m.deletingID
		m.deletingID = ""
		m.busy = true
		return func() tea.Msg {
			c, ok := m.client.(interface {
				DeleteSession(context.Context, string) error
			})
			if !ok {
				return result{err: fmt.Errorf("session deletion unsupported")}
			}
			ctx, cancel := context.WithTimeout(m.ctx, 35*time.Second)
			defer cancel()
			err := c.DeleteSession(ctx, id)
			return result{text: "session deleted; journal retained", err: err}
		}
	}
	switch key.String() {
	case "s":
		if m.current() != nil {
			m.busy = true
			return m.action("stop", "")
		}
	case "up":
		m.selected = max(0, m.selected-1)
	case "down":
		m.selected = max(0, min(len(m.sessions)-1, m.selected+1))
	case "enter":
		if m.current() != nil {
			m.projectView = false
			m.focusList = false
			m.input.Reset()
			m.watch()
			m.render()
		}
	case "ctrl+n", "n":
		if m.createProjectSession != nil {
			m.busy = true
			e := &createProjectExec{create: m.createProjectSession}
			return tea.Exec(e, func(err error) tea.Msg {
				r := result{err: err, text: "session created"}
				if err == nil && e.session != nil {
					r.sessionID = e.session.Id
				}
				return r
			})
		}
		m.creating = true
		m.input.SetValue(m.project.Workspace)
		m.notice = "Loading accounts…"
		return m.loadAccounts()
	case "delete", "d":
		if s := m.current(); s != nil {
			m.deletingID = s.Id
		}
	}
	return nil
}

func (m *model) projectScreen() string {
	var b strings.Builder
	p := m.project
	fmt.Fprintf(&b, "cxz · project · %s (%s)\nWorkspace: %s\nState: %s · ID: %s\n\n", pickerLabel(p.Name), pickerLabel(p.Alias), pickerLabel(p.Workspace), pickerLabel(p.State), p.Id)
	rows := max(1, m.height-10)
	start := max(0, m.selected-rows+1)
	if len(m.sessions) == 0 {
		b.WriteString("No sessions. Press n to create one.\n")
	}
	for i := start; i < min(len(m.sessions), start+rows); i++ {
		s := m.sessions[i]
		mark := " "
		if i == m.selected {
			mark = ">"
		}
		fmt.Fprintf(&b, "%s %.8s  %-13s %s · %s · %s\n", mark, s.Id, pickerLabel(s.State), pickerLabel(s.Agent), pickerLabel(s.Account), pickerLabel(s.Title))
	}
	b.WriteString("\n↑/↓ select · Enter open · n new · s stop · d delete · Ctrl+C detach\n")
	if m.deletingID != "" {
		fmt.Fprintf(&b, "Delete session %.8s? Active agent will stop. Journal retained. [y/N]\n", m.deletingID)
	}
	if m.busy {
		b.WriteString("Working…\n")
	}
	if m.creating {
		b.WriteString(m.input.View() + "\n")
	}
	b.WriteString(safeText(m.notice))
	lines := strings.Split(b.String(), "\n")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, max(1, m.width), "…")
	}
	return strings.Join(lines, "\n")
}
