package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

type accountWorkflow struct {
	ctx                              context.Context
	cancel                           context.CancelFunc
	in                               *io.PipeWriter
	reader                           *io.PipeReader
	updates                          chan tea.Msg
	input                            textinput.Model
	alias, provider, output, message string
	create, sending, canceling       bool
	started                          time.Time
	offset                           int
}
type workflowOutput struct {
	flow *accountWorkflow
	text string
}
type workflowDone struct {
	flow    *accountWorkflow
	session *api.Session
	err     error
}
type workflowWritten struct {
	flow *accountWorkflow
	err  error
}

func (f *accountWorkflow) close() {
	f.cancel()
	f.in.Close()
	f.reader.Close()
	f.input.Reset()
}
func (f *accountWorkflow) wait() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-f.updates:
			return msg
		case <-f.ctx.Done():
			return nil
		}
	}
}

type workflowWriter struct{ flow *accountWorkflow }

func (w workflowWriter) Write(p []byte) (int, error) {
	select {
	case w.flow.updates <- workflowOutput{w.flow, string(p)}:
		return len(p), nil
	case <-w.flow.ctx.Done():
		return 0, w.flow.ctx.Err()
	}
}

func (m *model) startAccountWorkflow(alias, provider, sessionID string, create bool) tea.Cmd {
	ctx, cancel := context.WithCancel(m.ctx)
	r, w := io.Pipe()
	f := &accountWorkflow{ctx: ctx, cancel: cancel, reader: r, in: w, updates: make(chan tea.Msg, 16), alias: alias, provider: provider, create: create, started: time.Now(), input: textinput.New()}
	f.input.EchoMode = textinput.EchoNone
	f.input.CharLimit = 8192
	f.input.Focus()
	m.workflow = f
	m.busy = true
	creator, login := m.createProjectSession, m.loginAccount
	projectID := ""
	if m.project != nil {
		projectID = m.project.Id
	}
	work := func() tea.Msg {
		defer r.Close()
		defer w.Close()
		out := workflowWriter{f}
		var s *api.Session
		var err error
		if create {
			s, err = creator(ctx, projectID, alias, r, out, out)
		} else {
			err = login(ctx, projectID, alias, sessionID, r, out, out)
		}
		// Completion uses the command return path so cancellation cannot swallow it.
		return workflowDone{f, s, err}
	}
	return tea.Batch(work, f.wait())
}

func (m *model) workflowKey(k tea.KeyMsg) tea.Cmd {
	f := m.workflow
	switch k.String() {
	case "esc", "ctrl+c":
		f.canceling = true
		f.message = "Canceling; waiting for operation to stop…"
		f.close()
		return nil
	case "pgup":
		f.offset += max(1, m.height-10)
		return nil
	case "pgdown":
		f.offset = max(0, f.offset-max(1, m.height-10))
		return nil
	case "ctrl+x":
		f.input.Reset()
		return nil
	case "enter", "ctrl+s":
		if f.provider != "claude" || f.sending || f.canceling || strings.TrimSpace(f.input.Value()) == "" {
			return nil
		}
		code := f.input.Value()
		f.input.Reset()
		f.sending = true
		f.message = "Submitting code…"
		return func() tea.Msg { _, err := io.WriteString(f.in, code+"\n"); return workflowWritten{f, err} }
	}
	if f.provider == "claude" && !f.sending && !f.canceling {
		var cmd tea.Cmd
		f.input, cmd = f.input.Update(k)
		return cmd
	}
	return nil
}

func (m *model) workflowScreen() string {
	f := m.workflow
	width := max(1, m.width-4)
	title := "Account login"
	if f.create {
		title = "Preparing agent session"
	}
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	rows := []string{brand.Render("cxz · " + title), providerLabel(f.provider) + " · " + pickerLabel(f.alias), "", accent.Render(fmt.Sprintf("%c %s · %s", frames[m.pulse%len(frames)], title, time.Since(f.started).Round(time.Second))), ""}
	// Strip provider control sequences: child processes never own this terminal.
	text := ansi.Hardwrap(safeText(ansi.Strip(f.output)), width, true)
	output := strings.Split(text, "\n")
	capacity := max(1, m.height-12)
	f.offset = min(f.offset, max(0, len(output)-capacity))
	end := len(output) - f.offset
	start := max(0, end-capacity)
	rows = append(rows, output[start:end]...)
	for len(rows) < 5+capacity {
		rows = append(rows, "")
	}
	rows = append(rows, "", muted.Render(f.message))
	if f.provider == "claude" {
		n := utf8.RuneCountInString(f.input.Value())
		digits := fmt.Sprintf("%03d", n)
		padding := len(digits) - len(fmt.Sprint(n))
		cursor := " "
		if m.pulse%2 == 0 && !f.sending && !f.canceling {
			cursor = accent.Render("▏")
		}
		rows = append(rows, "["+strings.Repeat("*", min(3, n))+strings.Repeat(" ", max(0, 3-n))+"]"+cursor+muted.Render(digits[:padding])+digits[padding:]+"; Ctrl+X clear · Enter submit code")
	} else {
		instruction := "Complete login in your browser using the URL and code above."
		if f.create {
			instruction = "Preparing this session; provider instructions appear above when needed."
		}
		rows = append(rows, muted.Render(instruction))
	}
	rows = append(rows, muted.Render("Esc cancel · PgUp/PgDn scroll · credentials are not added to chat history"))
	for len(rows) < m.height {
		rows = append(rows, "")
	}
	return screen(indentBlock(strings.Join(rows, "\n")), m.width, m.height)
}

func (m *model) loginSessions() []*api.Session {
	var out []*api.Session
	for _, s := range m.sessions {
		if s.Account == m.loginAlias && s.Agent == "claude" && (m.project == nil || s.ProjectId == m.project.Id) {
			out = append(out, s)
		}
	}
	return out
}

func (m *model) loginTargetKey(k tea.KeyMsg) tea.Cmd {
	sessions := m.loginSessions()
	switch k.String() {
	case "esc", "ctrl+q":
		m.loginChoosing = false
	case "up", "shift+tab":
		m.loginIndex = max(0, m.loginIndex-1)
	case "down", "tab":
		m.loginIndex = min(len(sessions), m.loginIndex+1)
	case "enter":
		if m.loginIndex == len(sessions) {
			if m.createProjectSession == nil {
				m.notice = "Session creation unavailable"
				return nil
			}
			m.loginChoosing = false
			return m.startAccountWorkflow(m.loginAlias, "claude", "", true)
		}
		s := sessions[m.loginIndex]
		if s.State != "stopped" && s.State != "failed" {
			m.notice = "Stop this session in the project view before logging in again."
			return nil
		}
		m.loginChoosing = false
		return m.startAccountWorkflow(m.loginAlias, "claude", s.Id, false)
	}
	return nil
}
