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

type ProjectCreator func(string, io.Reader, io.Writer, io.Writer) (*api.Session, error)
type AccountLogin func(string, io.Reader, io.Writer, io.Writer) error

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
	m.saveDraft()
	m.projectView = true
	m.sessions = ProjectSessions(m.sessions, m.project)
	m.selected = max(0, min(m.selected, len(m.sessions)-1))
	m.creating = false
	m.focusList = false
	m.focusApproval = false
	m.fullPermission = nil
	m.interruptKey = ""
	m.approvalOffset = 0
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
	create   func(io.Reader, io.Writer, io.Writer) (*api.Session, error)
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
	case "a":
		return m.openAccounts(false)
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
			m.restoreDraft()
			m.watch()
			m.render()
			return m.input.Focus()
		}
	case "ctrl+n", "n":
		if m.createProjectSession != nil {
			return m.openAccounts(true)
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
	p := m.project
	width := max(1, m.width-4)
	name := pickerLabel(p.Name)
	if name == "" {
		name = pickerLabel(p.Alias)
	}
	header := brand.Render("cxz · project") + "  /  " + strong.Render(name)
	meta := pickerLabel(p.Workspace)
	info := fmt.Sprintf("%s  ·  %s  ·  %d sessions", pickerLabel(p.Alias), pickerLabel(p.State), len(m.sessions))
	var rows []string
	rows = append(rows, clip(header, width), muted.Render(clip(meta, width)), accent.Render(clip(info, width)), "", strong.Render("Sessions")+"  "+muted.Render("newest first"))
	capacity := max(1, (m.height-13)/3)
	start := max(0, m.selected-capacity+1)
	if len(m.sessions) == 0 {
		rows = append(rows, "", strong.Render("No sessions yet"), muted.Render("Press n to choose an account and start a conversation."))
	}
	for i := start; i < min(len(m.sessions), start+capacity); i++ {
		s := m.sessions[i]
		title := pickerLabel(s.Title)
		if title == "" {
			title = "Untitled conversation"
		}
		mark := "  "
		style := strong
		if i == m.selected {
			mark = "› "
			style = selectedRow
		}
		rows = append(rows, style.Render(clip(mark+title, width)))
		handle := pickerLabel(safeText(s.Alias))
		if handle == "" {
			handle = fmt.Sprintf("%.8s", s.Id)
		}
		detail := fmt.Sprintf("  %-7s  %s · %s · %s", handle, pickerLabel(s.State), pickerLabel(s.Agent), pickerLabel(s.Account))
		rows = append(rows, blue.Render(clip(detail, width)), "")
	}
	if len(m.sessions) > capacity {
		rows = append(rows, muted.Render(fmt.Sprintf("  %d–%d of %d", start+1, min(len(m.sessions), start+capacity), len(m.sessions))))
	}
	footer := muted.Render("↑/↓ select · Enter open · n new · a accounts · s stop · d delete") + "\n" + muted.Render("Ctrl+C detach · agents keep running")
	status := pickerLabel(m.notice)
	if m.busy {
		status = "Working… " + status
	}
	if m.deletingID != "" {
		status = fmt.Sprintf("Delete session %.8s? [y/N]\nActive agent will stop. Journal retained.", m.deletingID)
	}
	if m.creating {
		status = m.input.View() + "\n" + status
	}
	footer += "\n" + warning.Render(status)
	gap := max(0, m.height-len(rows)-strings.Count(footer, "\n")-2)
	all := strings.Split(strings.Join(rows, "\n")+strings.Repeat("\n", gap+1)+footer, "\n")
	for i, line := range all {
		plain := ansi.Strip(line)
		if !strings.HasPrefix(plain, "› ") && !strings.HasPrefix(plain, "  ") {
			all[i] = "  " + line
		}
	}
	return screen(strings.Join(all, "\n"), m.width, m.height)
}
