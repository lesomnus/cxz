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
	"github.com/lesomnus/cxz/internal/agentview"
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
	hintOffset           int
	hintDismissed        bool
	renaming             bool
	renameBusy           bool
	renameID             string
	aliasInput           textinput.Model
	usageReports         map[string]string
	usageGeneration      map[string]uint64
	quotaWindows         []agentview.Window
	quotaState           string
	modelPicker          *modelPicker
	modelPickerEpoch     uint64
	report               *reportOverlay
	contextCapture       *contextCapture
	hiddenEvents         map[*api.Event]bool
	view                 viewport.Model
	focusList, creating  bool
	notice               string
	width, height        int
	events               map[string][]*api.Event
	cursor               map[string]uint64
	historyLoading       map[string]bool
	historyStart         map[string]uint64
	watchCancel          context.CancelFunc
	watchID              string
	watchEpoch           uint64
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
	focusApproval        bool
	approvalID           string
	approvalOffset       int
	interruptUntil       time.Time
	interruptKey         string
	approvalSent         map[string]bool
	fullPermission       map[string]string
	localReports         map[string]string
	historyTimes         []int64
	historyPositions     []float64 // stable journal coordinates, not loaded-line offsets
	restartConfirm       *restartConfirmation
	questionDialog       *questionDialog
	questionSeen         map[string]bool
	restartBusy          bool
	lastPromptStart      int
	lastPromptEnd        int
	latestPrompt         string
	pulse                int
	workingSince         int64
	backgroundHistory    map[string][]*api.Event
	backgroundLoading    map[string]bool
	backgroundErrors     map[string]string
	cursorOutput         *cursorWriter
	renderedResponses    map[*api.Event]renderedResponse
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
type pulseTick struct{}

func pulseTimer() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return pulseTick{} })
}

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
	m.cursorOutput = &cursorWriter{out: os.Stdout, keyboard: extendedKeyboard}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx), tea.WithMouseCellMotion(), tea.WithOutput(m.cursorOutput), tea.WithInput(keyboardInput(os.Stdin)))
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
func (m *model) Init() tea.Cmd { return tea.Batch(m.refresh(), timer(), textarea.Blink, pulseTimer()) }
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
	m.watchEpoch++
	epoch := m.watchEpoch
	after := m.cursor[id]
	last := s.LastSeq
	go func() {
		if after == 0 && last > 0 {
			start := uint64(0)
			if last > historyPageSize {
				start = last - historyPageSize
			}
			batch, err := m.client.History(ctx, &api.WatchRequest{SessionId: id, AfterSeq: start})
			if err != nil {
				m.program.Send(disconnected{id, err})
				return
			}
			for _, e := range batch.Events {
				after = max(after, e.Seq)
			}
			if ctx.Err() != nil {
				return
			}
			m.program.Send(historyPage{id: id, events: batch.Events, start: start, initial: true, epoch: epoch})
		}
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
	m.updateQuota()
	m.workingSince = 0
	follow := m.view.AtBottom()
	s := m.current()
	if s == nil {
		m.historyTimes = nil
		m.historyPositions = nil
		m.latestPrompt = ""
		m.lastPromptStart, m.lastPromptEnd = -1, -1
		m.view.SetContent(indentBlock(ansi.Hardwrap("No sessions. Ctrl+N creates one from an existing workspace directory.", max(1, m.view.Width-2), true)))
		if _, ok := m.localHelp[""]; ok {
			m.view.SetContent(m.localCommandView(""))
		}
		return
	}
	var lines []string
	var times []int64
	var sequences []uint64
	var sequence uint64
	promptBlock := -1
	m.latestPrompt = ""
	add := func(text string, stamp int64) {
		lines = append(lines, text)
		times = append(times, stamp)
		sequences = append(sequences, sequence)
	}
	var usage *api.Event
	var started int64
	replyIndex := -1
	helpAfter, showHelp := m.localHelp[s.Id]
	helped := false
	contextTurns := map[string]bool{}
	decisions := map[string]string{}
	toolCalls := map[string]agentview.ToolActivity{}
	toolResults := map[string]*api.Event{}
	backgroundTools := map[string]agentview.BackgroundTask{}
	for run, state := range m.backgroundStates() {
		for _, task := range state.Tasks {
			if task.ToolID != "" {
				backgroundTools[run+"/"+task.ToolID] = task
			}
		}
	}
	toolApprovals := map[string]*api.Event{}
	pairedApprovals := map[*api.Event]bool{}
	for _, e := range m.events[s.Id] {
		if e.Kind == "tool_call" && e.RequestId != "" && !question(e) && !m.hiddenEvents[e] {
			if activity, ok := agentview.ToolView(s.Agent, e.Text, e.Payload); ok {
				toolCalls[e.RunId+"/"+e.RequestId] = activity
			} else {
				toolCalls[e.RunId+"/"+e.RequestId] = agentview.ToolActivity{Kind: "tool", Description: e.Text}
			}
		}
		if e.Kind == "tool_result" && e.RequestId != "" {
			toolResults[e.RunId+"/"+e.RequestId] = e
		}
		if e.Kind == "approval_resolved" {
			decisions[e.RunId+"/"+e.RequestId] = e.Text
		}
	}
	for _, e := range m.events[s.Id] {
		if e.Kind != "approval" || question(e) {
			continue
		}
		root := fields(e.Payload)
		id := root.text("tool_use_id")
		if s.Agent == "codex" {
			id = root.object("params").text("itemId")
		}
		if id != "" {
			key := e.RunId + "/" + id
			if _, ok := toolCalls[key]; ok {
				toolApprovals[key] = e
				pairedApprovals[e] = true
			}
		}
	}
	for _, e := range m.events[s.Id] {
		if showHelp && !helped && e.Seq > helpAfter {
			add(m.localCommandView(s.Id), 0)
			helped = true
		}
		sequence = e.Seq
		if e.Kind == "input" {
			contextTurns[e.RunId] = strings.TrimSpace(e.Text) == "/context"
		}
		contextOutput := contextTurns[e.RunId] && (e.Kind == "input" || e.Kind == "assistant" || e.Kind == "turn_end")
		if e.Kind == "turn_end" {
			delete(contextTurns, e.RunId)
		}
		if contextOutput {
			continue
		}
		if m.hiddenEvents[e] {
			continue
		}
		if e.RunId == "" || e.RunId == s.RunId {
			switch e.Kind {
			case "input":
				m.workingSince = e.TimeMs
			case "turn_end":
				m.workingSince = 0
			case "state":
				if m.workingSince == 0 && e.Text == "working" {
					m.workingSince = e.TimeMs
				}
			}
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
					add(text, e.TimeMs)
				}
			}
			usage = nil
			started = 0
			replyIndex = -1
		} else {
			text := eventViewCached(m, s, e, max(1, m.view.Width))
			key := e.RunId + "/" + e.RequestId
			if e.Kind == "tool_call" {
				if activity, ok := toolCalls[key]; ok {
					state := toolInitialState(s.Agent, e)
					if approval := toolApprovals[key]; approval != nil {
						state = decisions[approval.RunId+"/"+approval.RequestId]
						if state == "" {
							state = "pending"
						}
						if state == "allowed" {
							state = "working"
						}
					}
					result := toolResults[key]
					if result != nil && s.Agent == "codex" {
						if final, ok := agentview.ToolView(s.Agent, result.Text, result.Payload); ok {
							activity = final
						}
					}
					text = indentBlock(toolActivityStateBody(activity, result, max(1, m.view.Width), state))
					if task, ok := backgroundTools[key]; ok {
						state = task.Status
						if task.Active {
							state = "working"
						}
						if !task.Active && (state == "working" || state == "running") {
							state = "pending"
						}
						text = indentBlock(toolActivityStateBody(activity, nil, max(1, m.view.Width), state) + " · background")
					}
				}
			}
			if e.Kind == "tool_result" {
				if _, ok := toolCalls[key]; ok {
					continue
				}
			}
			if e.Kind == "approval" {
				if pairedApprovals[e] {
					continue
				}
				state := decisions[e.RunId+"/"+e.RequestId]
				if state == "" {
					if m.hiddenAutoApproval(s, e) {
						continue
					}
					state = "requested"
				}
				text = approvalLine(s, e, state, max(1, m.view.Width))
			}
			if text != "" {
				add(text, e.TimeMs)
				if e.Kind == "input" {
					promptBlock = len(lines) - 1
					m.latestPrompt = e.Text
				}
				if e.Kind == "assistant" {
					replyIndex = len(lines) - 1
				}
			}
		}
		if hint := authHint(s, e); hint != "" {
			add(indentBlock(warning.Render(ansi.Hardwrap(safeText(hint), max(1, m.view.Width-2), true))), e.TimeMs)
		}
	}
	if showHelp && !helped {
		add(m.localCommandView(s.Id), 0)
	}
	if len(lines) == 0 {
		add(indentBlock(muted.Render(ansi.Hardwrap("Start a conversation\n\nDescribe a task below. Messages and tool activity will appear here.\nStopped session? Ctrl+R resumes the agent.", max(1, m.view.Width-2), true))), 0)
	}
	if m.activeWork() {
		add("  ", 0)
	}
	m.historyTimes = nil
	m.historyPositions = nil
	m.lastPromptStart, m.lastPromptEnd = -1, -1
	for i, block := range lines {
		if i > 0 {
			m.historyTimes = append(m.historyTimes, 0)
			m.historyPositions = append(m.historyPositions, float64(sequences[i]))
		}
		start := len(m.historyTimes)
		rows := strings.Split(block, "\n")
		for row := range rows {
			m.historyTimes = append(m.historyTimes, times[i])
			m.historyPositions = append(m.historyPositions, float64(sequences[i])+float64(row)/float64(len(rows)))
		}
		if i == promptBlock {
			m.lastPromptStart = start
			m.lastPromptEnd = len(m.historyTimes)
		}
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
	if kind == "allow" || kind == "deny" || kind == "answer" {
		return m.replyApproval(m.selectedApproval(), kind != "deny", text, false)
	}
	s := m.current()
	if s == nil {
		return nil
	}
	id, run := s.Id, s.RunId
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		ctrl := &api.Control{SessionId: id, RunId: run, ClientId: core.ID()}
		var e error
		var receipt *api.Receipt
		switch kind {
		case "send":
			receipt, e = m.client.Send(ctx, &api.Input{SessionId: id, RunId: run, ClientId: ctrl.ClientId, Text: text})
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
	if p := m.report; p != nil {
		if s := m.current(); m.projectView || m.accountView || s != nil && (s.Id != p.id || s.RunId != p.run) {
			m.report = nil
		}
	}
	if c := m.contextCapture; c != nil {
		if s := m.current(); s == nil || s.Id != c.id || s.RunId != c.run {
			m.contextCapture = nil
		}
	}
	switch v := msg.(type) {
	case contextSent:
		if c := m.contextCapture; c != nil && c.request == v.request && v.err != nil {
			c.report.text = "Context unavailable: " + v.err.Error()
			m.contextCapture = nil
		}
		return m, nil
	case modelCatalogLoaded:
		return m, m.acceptModelCatalog(v)
	case historyPage:
		m.applyHistoryPage(v)
		background := m.loadBackgroundHistory(v)
		if background != nil {
			if m.backgroundLoading == nil {
				m.backgroundLoading = map[string]bool{}
			}
			m.backgroundLoading[v.id] = true
		}
		if v.err == nil && m.view.TotalLineCount() <= m.view.Height {
			return m, tea.Batch(m.loadOlderHistory(), background)
		}
		return m, background
	case backgroundHistory:
		if v.epoch != 0 && v.epoch != m.watchEpoch {
			return m, nil
		}
		if m.backgroundHistory == nil {
			m.backgroundHistory = map[string][]*api.Event{}
		}
		if m.backgroundErrors == nil {
			m.backgroundErrors = map[string]string{}
		}
		m.backgroundHistory[v.id] = v.events
		delete(m.backgroundLoading, v.id)
		delete(m.backgroundErrors, v.id)
		if v.err != nil {
			m.backgroundErrors[v.id] = v.err.Error()
		}
		m.render()
		return m, nil
	case pulseTick:
		m.pulse++
		return m, pulseTimer()
	case approvalResult:
		if d := m.questionDialog; d != nil && d.id == v.id && d.run == v.run && d.request == v.request {
			if v.err == nil {
				m.questionDialog = nil
			} else {
				d.sending = false
				d.message = "Answer failed: " + v.err.Error() + ". Not retried automatically."
				delete(m.approvalSent, v.id+"/"+v.run+"/"+v.request)
			}
		}
		if v.err != nil {
			delete(m.fullPermission, v.id)
			m.notice = "Approval failed; not retried. " + v.err.Error()
		} else {
			for _, s := range m.sessions {
				if s.Id == v.id && s.RunId == v.run {
					var pending []*api.Event
					for _, p := range s.Pending {
						if p.RequestId != v.request {
							pending = append(pending, p)
						}
					}
					s.Pending = pending
				}
			}
			if !v.automatic {
				m.notice = "Approval decision sent"
			}
		}
		if m.focusApproval {
			m.focusApproval = false
			m.input.Focus()
		}
		m.resize()
		m.render()
		return m, tea.Batch(m.refresh(), m.autoApprove())
	case tea.MouseMsg:
		if m.questionDialog != nil {
			if v.Button == tea.MouseButtonWheelUp {
				m.questionDialog.offset -= 3
			}
			if v.Button == tea.MouseButtonWheelDown {
				m.questionDialog.offset += 3
			}
			return m, nil
		}
		if m.restartConfirm != nil {
			return m, m.restartMouse(v)
		}
		if m.report != nil {
			if v.Button == tea.MouseButtonWheelUp {
				m.report.offset = max(0, m.report.offset-3)
			}
			if v.Button == tea.MouseButtonWheelDown {
				m.report.offset += 3
			}
			return m, nil
		}
		if m.modelPicker != nil {
			return m, nil
		}
		if m.focusApproval && v.Y >= m.view.Height && v.Y < m.height-m.input.Height()-3 {
			switch v.Button {
			case tea.MouseButtonWheelUp:
				m.approvalOffset = max(0, m.approvalOffset-3)
			case tea.MouseButtonWheelDown:
				m.approvalOffset += 3
			}
			return m, nil
		}
		if !m.projectView && !m.accountView && v.Y < m.view.Height {
			m.view, _ = m.view.Update(v)
			return m, m.loadOlderHistory()
		}
		return m, nil
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
		if p := m.report; p != nil && p.id == v.id && p.title == "/usage" && p.generation == v.generation {
			p.text = m.usageReports[v.id]
		}
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
			m.fullPermission = nil
			m.notice = "daemon disconnected; reconnecting (commands are not retried)"
			return m, nil
		}
		oldApproval := ""
		if p := m.selectedApproval(); p != nil && m.focusApproval {
			oldApproval = p.RequestId
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
		if oldApproval != "" {
			found := false
			if s := m.current(); s != nil {
				for _, p := range s.Pending {
					if p.RequestId == oldApproval {
						found = true
					}
				}
			}
			if !found {
				m.approvalOffset = 0
				m.focusApproval = false
				m.input.Focus()
				m.notice = "Selected approval resolved; review the next request before deciding"
			}
		}
		for _, s := range m.sessions {
			if m.fullPermission[s.Id] != "" && (m.fullPermission[s.Id] != s.RunId || !permissionState(s.State)) {
				delete(m.fullPermission, s.Id)
			}
		}
		if m.selectedApproval() == nil {
			m.approvalOffset = 0
			if m.focusApproval {
				m.input.Focus()
			}
			m.focusApproval = false
		}
		m.resize()
		m.render()
		m.syncQuestion()
		return m, m.autoApprove()
	case received:
		if v.event.Seq > m.cursor[v.id] {
			m.captureContext(v.id, v.event)
			if s := m.current(); s != nil && s.Id == v.id && (v.event.Kind == "turn_end" || v.event.Kind == "input") {
				m.interruptUntil = time.Time{}
				m.interruptKey = ""
			}
			m.cursor[v.id] = v.event.Seq
			m.events[v.id] = append(m.events[v.id], v.event)
			if p := m.modelPicker; p != nil && !p.loading && p.id == v.id && p.run == v.event.RunId && v.event.Kind == "models" {
				var catalog modelCatalog
				if json.Unmarshal(v.event.Payload, &catalog) == nil {
					selected := ""
					options := p.options()
					if p.selected < len(options) {
						selected = options[p.selected]
					}
					p.catalog = &catalog
					p.selected = 0
					for i, option := range p.options() {
						if option == selected {
							p.selected = i
							break
						}
					}
				}
			}
			if s := m.current(); s != nil && s.Id == v.id {
				m.render()
			}
		}
	case disconnected:
		if c := m.contextCapture; c != nil && c.id == v.id {
			c.report.text = "Disconnected while querying context. Reopen /context after reconnecting."
			m.contextCapture = nil
		}
		delete(m.fullPermission, v.id)
		if m.watchID == v.id {
			m.watchID = ""
			m.notice = "event connection lost; reconnecting from saved cursor"
		}
	case restartFinished:
		m.restartBusy = false
		if v.err != nil {
			m.notice = v.err.Error()
		} else {
			m.notice = "Agent restarted · same session · manual approval"
		}
		return m, m.refresh()
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
		if m.questionDialog != nil {
			return m, m.questionKey(v)
		}
		if m.restartConfirm != nil {
			return m, m.restartKey(v)
		}
		if m.report != nil {
			return m, m.reportKey(v)
		}
		if m.modelPicker != nil {
			return m, m.modelPickerKey(v)
		}
		if m.accountView {
			return m, m.accountKey(v)
		}
		if m.renaming {
			return m, m.renameKey(v)
		}
		if !m.projectView && !m.creating && !m.focusApproval {
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
		if m.focusApproval && v.String() != "tab" && v.String() != "shift+tab" && v.String() != "ctrl+c" {
			return m, m.approvalKey(v)
		}
		switch v.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "pgup", "pgdown":
			m.view, _ = m.view.Update(v)
			return m, m.loadOlderHistory()
		case "ctrl+end":
			m.view.GotoBottom()
			return m, nil
		case "ctrl+home":
			m.view.GotoTop()
			return m, m.loadOlderHistory()
		case "tab", "shift+tab":
			if m.creating {
				if len(m.accounts) > 0 {
					step := 1
					if v.String() == "shift+tab" {
						step = len(m.accounts) - 1
					}
					m.accountIndex = (m.accountIndex + step) % len(m.accounts)
				}
				m.notice = m.accountNotice()
				return m, nil
			}
			return m, m.cycleFocus(v.String() == "shift+tab")
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
				return m, m.confirmInterrupt(time.Now())
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
			if !m.focusList && !m.creating {
				m.input.InsertString("\n")
				m.resize()
				return m, nil
			}
			fallthrough
		case "ctrl+s":
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
			if text == "/answer" {
				return m, m.openQuestion(m.selectedApproval())
			}
			if strings.HasPrefix(text, "/answer ") {
				return m, m.action("answer", strings.TrimPrefix(text, "/answer "))
			}
			localName := strings.Fields(text)[0]
			if localName == "/background" {
				m.openReport("/background", "")
				return m, nil
			}
			if localName == "/restart" {
				return m, m.restartCommand(text)
			}
			if localName == "/model" || localName == "/effort" {
				return m, m.modelCommand(text)
			}
			if localName == "/permission" {
				return m, m.permissionCommand(text)
			}
			if localName == "/approval" {
				m.approvalDetails()
				return m, nil
			}
			if localName == "/usage" {
				return m, m.loadUsage()
			}
			if localName == "/details" {
				m.toolDetails()
				return m, nil
			}
			if localName == "/context" {
				return m, m.contextCommand()
			}
			if localName == "/compact" {
				return m, m.compactCommand(text)
			}
			if localName == "/help" {
				m.openReport(text, "")
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
	m.anchorCursor()
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
