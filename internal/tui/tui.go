package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/resource"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type model struct {
	project              *api.Project
	projectView          bool
	deletingID           string
	busy                 bool
	createProjectSession ProjectCreator
	ctx                  context.Context
	client               api.SessionsClient
	sessions             []*api.Session
	selected             int
	input                textarea.Model
	drafts               map[string]string
	localHelp            map[string]uint64
	localOutput          map[string]string
	hintSelected         int
	hintDismissed        bool
	renaming             bool
	renameBusy           bool
	renameID             string
	aliasInput           textinput.Model
	usageReports         map[string]string
	usageGeneration      map[string]uint64
	view                 viewport.Model
	focusList, creating  bool
	notice               string
	width, height        int
	events               map[string][]*api.Event
	cursor               map[string]uint64
	watchCancel          context.CancelFunc
	watchID              string
	wantID               string
	accounts             []*resource.Account
	accountIndex         int
	accountView          bool
	accountAdding        bool
	accountChoosing      bool
	accountLoading       bool
	accountAgent         string
	accountField         int
	accountAlias         textinput.Model
	accountName          textinput.Model
	accountSearch        textinput.Model
	accountSearching     bool
	accountService       resource.AccountServiceClient
	loginAccount         AccountLogin
	program              *tea.Program
}
type listing struct {
	project  *api.Project
	sessions []*api.Session
	err      error
}
type received struct {
	id    string
	event *api.Event
}
type disconnected struct {
	id  string
	err error
}
type result struct {
	text      string
	err       error
	sessionID string
}
type tick time.Time
type accountListing struct {
	accounts []*resource.Account
	err      error
}

func (m *model) accountNotice() string {
	if len(m.accounts) == 0 {
		return "No registered accounts. Run cxz account add codex NAME, then cxz account login NAME."
	}
	a := m.accounts[m.accountIndex]
	return "Account: " + a.GetAlias() + " · " + a.GetAgent() + " · " + a.GetAuthBackend() + " (Tab changes; Enter creates)"
}
func (m *model) loadAccounts() tea.Cmd {
	return func() tea.Msg {
		service := m.accountService
		if service == nil {
			if c, ok := m.client.(*resourceclient.Client); ok {
				service = c.Accounts
			}
		}
		if service == nil {
			return accountListing{}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		var all []*resource.Account
		after := ""
		for {
			p, err := service.List(ctx, resource.AccountListRequest_builder{Size: 200, After: after}.Build())
			if err != nil {
				return accountListing{err: err}
			}
			all = append(all, p.GetItems()...)
			after = p.GetNext()
			if after == "" {
				return accountListing{accounts: all}
			}
		}
	}
}

func Run(ctx context.Context, c api.SessionsClient) error {
	return RunSelected(ctx, c, "")
}
func RunSelected(ctx context.Context, c api.SessionsClient, id string) error {
	var project *api.Project
	if id != "" {
		s, err := c.Get(ctx, &api.SessionRef{Id: id})
		if err != nil {
			return err
		}
		project = &api.Project{Id: s.ProjectId, Name: s.ProjectName, Alias: s.ProjectAlias, Workspace: s.Workspace}
	}
	return RunProject(ctx, c, project, id, nil)
}

func RunProject(ctx context.Context, c api.SessionsClient, project *api.Project, id string, create ProjectCreator, login ...AccountLogin) error {
	input := newComposer()
	m := &model{ctx: ctx, client: c, input: input, view: viewport.New(80, 15), events: map[string][]*api.Event{}, cursor: map[string]uint64{}, width: 100, height: 30, wantID: id}
	m.project = project
	m.projectView = project != nil && id == ""
	m.createProjectSession = create
	if resources, ok := c.(*resourceclient.Client); ok {
		m.accountService = resources.Accounts
	}
	if len(login) > 0 {
		m.loginAccount = login[0]
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	m.program = p
	_, e := p.Run()
	if m.watchCancel != nil {
		m.watchCancel()
	}
	return e
}
func (m *model) refresh() tea.Cmd {
	projectID := ""
	if m.project != nil {
		projectID = m.project.Id
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Second)
		defer cancel()
		v, e := m.client.List(ctx, &api.Empty{})
		if e != nil {
			return listing{err: e}
		}
		var p *api.Project
		if projectID != "" {
			if ps, err := m.client.Projects(ctx, &api.Empty{}); err == nil {
				for _, candidate := range ps.Projects {
					if candidate.Id == projectID {
						p = candidate
						break
					}
				}
			}
		}
		return listing{sessions: v.Sessions, project: p}
	}
}
func timer() tea.Cmd           { return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tick(t) }) }
func (m *model) Init() tea.Cmd { return tea.Batch(m.refresh(), timer(), textarea.Blink) }
func (m *model) current() *api.Session {
	if len(m.sessions) == 0 {
		return nil
	}
	m.selected = min(m.selected, len(m.sessions)-1)
	return m.sessions[m.selected]
}
func (m *model) watch() {
	if m.projectView || m.program == nil {
		return
	}
	s := m.current()
	if s == nil || m.watchID == s.Id {
		return
	}
	if m.watchCancel != nil {
		m.watchCancel()
	}
	id := s.Id
	ctx, cancel := context.WithCancel(m.ctx)
	m.watchCancel = cancel
	m.watchID = id
	after := m.cursor[id]
	go func() {
		stream, e := m.client.Watch(ctx, &api.WatchRequest{SessionId: id, AfterSeq: after})
		if e == nil {
			for {
				v, err := stream.Recv()
				if err != nil {
					e = err
					break
				}
				m.program.Send(received{id, v})
			}
		}
		if ctx.Err() == nil {
			m.program.Send(disconnected{id, e})
		}
	}()
}
func (m *model) render() {
	follow := m.view.AtBottom()
	s := m.current()
	if s == nil {
		m.view.SetContent(indentBlock(ansi.Hardwrap("No sessions. Ctrl+N creates one from an existing workspace directory.", max(1, m.view.Width-2), true)))
		if _, ok := m.localHelp[""]; ok {
			m.view.SetContent(m.localCommandView(""))
		}
		return
	}
	var lines []string
	var usage *api.Event
	var started int64
	replyIndex := -1
	helpAfter, showHelp := m.localHelp[s.Id]
	helped := false
	for _, e := range m.events[s.Id] {
		if showHelp && !helped && e.Seq > helpAfter {
			lines = append(lines, m.localCommandView(s.Id))
			helped = true
		}
		if e.Kind == "input" {
			started = e.TimeMs
			usage = nil
			replyIndex = -1
		}
		if e.Kind == "usage" && e.Text == "thread/tokenUsage/updated" {
			usage = e
		}
		if e.Kind == "turn_end" {
			if text := turnSummary(e, usage, started, max(1, m.view.Width)); text != "" {
				if replyIndex >= 0 {
					lines[replyIndex] += "\n\n" + text
				} else {
					lines = append(lines, text)
				}
			}
			usage = nil
			started = 0
			replyIndex = -1
		} else {
			if text := eventView(s, e, max(1, m.view.Width)); text != "" {
				lines = append(lines, text)
				if e.Kind == "assistant" {
					replyIndex = len(lines) - 1
				}
			}
		}
		if hint := authHint(s, e); hint != "" {
			lines = append(lines, indentBlock(warning.Render(ansi.Hardwrap(safeText(hint), max(1, m.view.Width-2), true))))
		}
	}
	if showHelp && !helped {
		lines = append(lines, m.localCommandView(s.Id))
	}
	if len(lines) == 0 {
		lines = append(lines, indentBlock(muted.Render(ansi.Hardwrap("Start a conversation\n\nDescribe a task below. Messages and tool activity will appear here.\nStopped session? Ctrl+R resumes the agent.", max(1, m.view.Width-2), true))))
	}
	m.view.SetContent(strings.Join(lines, "\n\n"))
	if follow {
		m.view.GotoBottom()
	}
}

// Agent/tool output is untrusted terminal data, not terminal instructions.
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 32 && r != 127 {
			return r
		}
		return -1
	}, ansi.Strip(s))
}
func (m *model) action(kind, text string) tea.Cmd {
	s := m.current()
	if s == nil {
		return nil
	}
	id, run := s.Id, s.RunId
	request := ""
	if len(s.Pending) > 0 {
		request = s.Pending[0].RequestId
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		ctrl := &api.Control{SessionId: id, RunId: run, ClientId: core.ID()}
		var e error
		var receipt *api.Receipt
		switch kind {
		case "send":
			receipt, e = m.client.Send(ctx, &api.Input{SessionId: id, RunId: run, ClientId: ctrl.ClientId, Text: text})
		case "allow", "deny", "answer":
			if request == "" {
				return result{err: fmt.Errorf("no pending approval")}
			}
			if kind == "answer" {
				var a map[string]string
				if json.Unmarshal([]byte(text), &a) != nil {
					return result{err: fmt.Errorf("/answer requires a JSON string map")}
				}
			}
			receipt, e = m.client.Reply(ctx, &api.Answer{SessionId: id, RunId: run, ClientId: ctrl.ClientId, RequestId: request, Allow: kind != "deny", AnswersJson: text})
		case "interrupt":
			receipt, e = m.client.Interrupt(ctx, ctrl)
		case "resume":
			_, e = m.client.Resume(ctx, ctrl)
		case "stop":
			receipt, e = m.client.Stop(ctx, ctrl)
		}
		message := kind
		if receipt != nil {
			message += " · " + receipt.Status
		}
		return result{text: message, err: e}
	}
}
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case renameResult:
		m.renameBusy = false
		if v.err != nil {
			m.notice = v.err.Error()
			return m, nil
		}
		for _, s := range m.sessions {
			if s.Id == v.id {
				s.Alias = v.alias
			}
		}
		m.renaming = false
		m.focusList = true
		m.notice = "alias updated"
		return m, m.refresh()
	case usageLoaded:
		if m.usageGeneration[v.id] != v.generation {
			return m, nil
		}
		m.usageReports[v.id] = v.text
		if v.err != nil {
			m.usageReports[v.id] = "Usage unavailable: " + v.err.Error()
		}
		m.render()
		return m, nil
	case accountListing:
		if !m.creating && !m.accountView {
			return m, nil
		}
		m.accountLoading = false
		if v.err != nil {
			m.notice = v.err.Error()
			return m, nil
		}
		alias := ""
		if choices := m.accountChoices(); len(choices) > m.accountIndex {
			alias = choices[m.accountIndex].GetAlias()
		}
		m.accounts = v.accounts
		m.accountIndex = 0
		for i, a := range m.accountChoices() {
			if a.GetAlias() == alias {
				m.accountIndex = i
			}
		}
		m.notice = m.accountNotice()
		if m.accountView {
			m.notice = ""
		}
		return m, nil
	case accountSaved:
		m.busy = false
		if v.err != nil {
			m.notice = v.err.Error()
			return m, nil
		}
		m.accountAdding = false
		m.accountSearch.Reset()
		m.accounts = []*resource.Account{v.account}
		m.accountIndex = 0
		m.notice = "Account registered. Press l to log in."
		m.accountLoading = true
		return m, m.loadAccounts()
	case accountLoggedIn:
		m.busy = false
		m.notice = "Login completed."
		if v.err != nil {
			m.notice = v.err.Error()
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.resize()
		m.render()
	case tick:
		m.watch()
		return m, tea.Batch(timer(), m.refresh())
	case listing:
		if v.err != nil {
			m.notice = "daemon disconnected; reconnecting (commands are not retried)"
			return m, nil
		}
		old := ""
		if s := m.current(); s != nil {
			old = s.Id
		}
		if m.wantID != "" {
			old = m.wantID
		}
		if v.project != nil {
			m.project = v.project
		}
		m.sessions = ProjectSessions(v.sessions, m.project)
		for i, s := range m.sessions {
			if s.Id == old {
				m.selected = i
				m.wantID = ""
			}
		}
		if strings.Contains(m.notice, "reconnecting") {
			m.notice = "connected"
		}
		m.watch()
		m.render()
	case received:
		if v.event.Seq > m.cursor[v.id] {
			m.cursor[v.id] = v.event.Seq
			m.events[v.id] = append(m.events[v.id], v.event)
			if len(m.events[v.id]) > 2000 {
				m.events[v.id] = m.events[v.id][len(m.events[v.id])-2000:]
			}
			if s := m.current(); s != nil && s.Id == v.id {
				m.render()
			}
		}
	case disconnected:
		if m.watchID == v.id {
			m.watchID = ""
			m.notice = "event connection lost; reconnecting from saved cursor"
		}
	case result:
		m.busy = false
		if v.err != nil {
			m.notice = v.err.Error()
		} else {
			m.notice = v.text
			if v.sessionID != "" {
				m.wantID = v.sessionID
				m.projectView = false
				m.accountView = false
			}
		}
		return m, m.refresh()
	case tea.KeyMsg:
		if m.accountView {
			return m, m.accountKey(v)
		}
		if m.renaming {
			return m, m.renameKey(v)
		}
		if !m.projectView && !m.creating {
			if handled, cmd := m.commandKey(v); handled {
				return m, cmd
			}
		}
		if v.String() == "ctrl+q" {
			m.backToProject()
			return m, m.refresh()
		}
		if m.projectView && !m.creating {
			return m, m.projectKey(v)
		}
		switch v.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "pgup", "pgdown":
			m.view, _ = m.view.Update(v)
			return m, nil
		case "tab":
			if m.creating {
				if len(m.accounts) > 0 {
					m.accountIndex = (m.accountIndex + 1) % len(m.accounts)
				}
				m.notice = m.accountNotice()
				return m, nil
			}
			m.focusList = !m.focusList
			if m.focusList {
				m.input.Blur()
				return m, nil
			}
			return m, m.input.Focus()
		case "r":
			if m.focusList {
				return m, m.startRename()
			}
		case "ctrl+n":
			m.creating = true
			m.focusList = false
			m.input.SetValue("")
			m.input.Placeholder = "absolute workspace path; Enter creates, Esc cancels"
			m.accounts = nil
			m.accountIndex = 0
			m.notice = "Loading registered accounts…"
			return m, m.loadAccounts()
		case "esc":
			if !m.creating {
				return m, nil
			}
			m.creating = false
			m.input.SetValue("")
			m.input.Placeholder = "message"
			return m, nil
		case "f2":
			return m, m.action("allow", "")
		case "f3":
			return m, m.action("deny", "")
		case "f4":
			return m, m.action("interrupt", "")
		case "ctrl+r":
			return m, m.action("resume", "")
		case "ctrl+x":
			m.input.Reset()
			return m, nil
		case "alt+enter", "ctrl+j":
			if !m.focusList && !m.creating {
				m.input.InsertString("\n")
				m.resize()
			}
			return m, nil
		case "up", "down":
			if m.focusList && len(m.sessions) > 0 {
				m.saveDraft()
				if v.String() == "up" {
					m.selected = (m.selected + len(m.sessions) - 1) % len(m.sessions)
				} else {
					m.selected = (m.selected + 1) % len(m.sessions)
				}
				m.restoreDraft()
				m.watch()
				m.render()
				return m, nil
			}
		case "enter":
			if m.focusList {
				m.focusList = false
				return m, m.input.Focus()
			}
			text := strings.TrimSpace(m.input.Value())
			if m.creating && m.project != nil {
				text = m.project.Workspace
			}
			if text == "" {
				return m, nil
			}
			m.input.SetValue("")
			if m.creating {
				if len(m.accounts) == 0 {
					m.notice = m.accountNotice()
					return m, nil
				}
				a := m.accounts[m.accountIndex]
				kind, account := a.GetAgent(), a.GetAlias()
				m.creating = false
				m.input.Placeholder = "message"
				return m, func() tea.Msg {
					path, e := filepath.Abs(text)
					if e != nil {
						return result{err: e}
					}
					ctx, cancel := context.WithTimeout(m.ctx, 30*time.Minute)
					defer cancel()
					if os.Getenv("CXZ_PROJECT_ID") != "" {
						s, e := m.client.Create(ctx, &api.CreateRequest{Workspace: path, Agent: kind, Model: settings.From(m.ctx).Model(kind), ClientId: core.ID(), Account: account})
						if e != nil {
							return result{err: e}
						}
						return result{text: "session created", sessionID: s.Id}
					}
					path, e = dockerx.EnginePath(path)
					if e != nil {
						return result{err: e}
					}
					s, e := m.client.Open(ctx, &api.ProjectRequest{Workspace: path, Agent: kind, Model: settings.From(m.ctx).Model(kind), NewSession: true, ClientId: core.ID(), Account: account})
					if e != nil {
						return result{err: e}
					}
					return result{text: "session created", sessionID: s.Id}
				}
			}
			if strings.HasPrefix(text, "/answer ") {
				return m, m.action("answer", strings.TrimPrefix(text, "/answer "))
			}
			localName := strings.Fields(text)[0]
			if localName == "/usage" {
				return m, m.loadUsage()
			}
			if localName == "/help" || localName == "/context" || localName == "/compact" {
				id := ""
				if s := m.current(); s != nil {
					id = s.Id
				}
				if m.localHelp == nil {
					m.localHelp = map[string]uint64{}
				}
				m.localHelp[id] = m.cursor[id]
				if m.localOutput == nil {
					m.localOutput = map[string]string{}
				}
				m.localOutput[id] = localName
				m.hintDismissed = false
				m.resize()
				m.render()
				m.view.GotoBottom()
				return m, nil
			}
			if text == "/stop" {
				return m, m.action("stop", "")
			}
			return m, m.action("send", text)
		}
	}
	var cmd tea.Cmd
	if m.accountView {
		if m.accountSearching {
			m.accountSearch, cmd = m.accountSearch.Update(msg)
		}
		if m.accountAdding && m.accountField == 1 {
			m.accountAlias, cmd = m.accountAlias.Update(msg)
		}
		if m.accountAdding && m.accountField == 2 {
			m.accountName, cmd = m.accountName.Update(msg)
		}
		return m, cmd
	}
	if m.renaming {
		m.aliasInput, cmd = m.aliasInput.Update(msg)
		return m, cmd
	}
	if !m.focusList {
		m.input, cmd = m.input.Update(msg)
	}
	m.resize()
	if k, ok := msg.(tea.KeyMsg); ok && (k.String() == "pgup" || k.String() == "pgdown") {
		m.view, _ = m.view.Update(msg)
	}
	return m, cmd
}
func (m *model) View() string {
	if m.width > 0 && (m.width < 40 || m.height < 14) {
		return screen("cxz\nResize terminal to 40 × 14 or larger.\nCtrl+C detach", m.width, m.height)
	}
	if m.accountView {
		return m.accountScreen()
	}
	if m.projectView {
		return m.projectScreen()
	}
	return m.sessionScreen()
}

func authHint(s *api.Session, e *api.Event) string {
	if e.Kind != "diagnostic" && e.Kind != "turn_end" && e.Kind != "stderr" {
		return ""
	}
	text := strings.ToLower(e.Text + " " + string(e.Payload))
	for _, needle := range []string{"not logged in", "unauthorized", "authentication", "login required", "401"} {
		if strings.Contains(text, needle) && s.ProjectId != "" {
			return fmt.Sprintf("Authentication may be required. Stop the session, run cxz account login --project %s %s, then cxz session resume %s. Failed prompts are not resent.", s.ProjectId, s.Account, s.Id)
		}
	}
	return ""
}
