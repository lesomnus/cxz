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
	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"os"
	"strings"
	"time"
)

type model struct {
	textSelection           *transcriptSelection
	codeButtons             []codeButton
	codeHover               *codeButton
	recordingError          string
	recordingTask           *debugSave
	debugRecorder           *debugRecorder
	recordingPending        *debugArchive
	recordingSaving         bool
	lastRecording           string
	toolSelector            *toolSelector
	noticeLogs              []noticeLog
	lastLoggedNotice        string
	filePreview             *filePreview
	pendingInputs           map[string][]*api.Event
	promptSpans             []promptSpan
	inlineDismissed         string
	redactDialog            *redactDialog
	redactions              map[string]*redaction
	redactSending           bool
	redactStore             secretFiles
	pathHints               *pathHints
	pathHintGeneration      uint64
	pathHintDismissed       string
	terminals               map[string]*terminalPanel
	activityID              string
	lastUIInput             time.Time
	lastActivityReport      time.Time
	project                 *api.Project
	resourcesWatching       bool
	resourceWatchCancel     context.CancelFunc
	resourceWatchGeneration uint64
	resourceWatchSignature  string
	resourceWatchStarted    time.Time
	resourceRefreshPending  bool
	resourceRefreshRunning  bool
	resourceRefreshAgain    bool
	resourceRetryDelay      time.Duration
	lastResourceRefresh     time.Time
	contextReports          map[string]contextReportSnapshot
	wisp                    *containerterm.WispPool
	terminalWidth           int
	panelFocus              bool
	panelWantKey            string
	panelWantConnection     string
	accountConnection       string
	creationConnection      string
	accountRequest          uint64
	panelIndex              int
	panelHoverY             int // Screen row; zero means no hovered item.
	panelProjects           []*api.Project
	allSessions             []*api.Session
	panelError              string
	projectView             bool
	deletingID              string
	busy                    bool
	createProjectSession    ProjectCreator
	ctx                     context.Context
	client                  api.SessionsClient
	sessions                []*api.Session
	selected                int
	input                   textarea.Model
	drafts                  map[string]string
	localHelp               map[string]uint64
	localOutput             map[string]string
	hintSelected            int
	hintOffset              int
	hintDismissed           bool
	renaming                bool
	renameBusy              bool
	renameID                string
	aliasInput              textinput.Model
	usageReports            map[string]string
	usageGeneration         map[string]uint64
	accountQuotas           map[string][]agentview.Window
	quotaWindows            []agentview.Window
	quotaState              string
	modelPicker             *modelPicker
	modelPickerEpoch        uint64
	settingsPage            *settingsPage
	memoryPage              *memoryPage
	report                  *reportOverlay
	contextCapture          *contextCapture
	hiddenEvents            map[*api.Event]bool
	view                    viewport.Model
	focusList, creating     bool
	notice                  string
	width, height           int
	events                  map[string][]*api.Event
	cursor                  map[string]uint64
	historyLoading          map[string]uint64 // End sequence of each in-flight older page.
	historyStart            map[string]uint64
	historyOpening          map[string]bool // Keep loading through pages containing only bookkeeping events.
	historyShimmer          *historyShimmer
	watchCancel             context.CancelFunc
	watchID                 string
	watchEpoch              uint64
	wantID                  string
	accounts                []*resource.Account
	accountIndex            int
	accountView             bool
	accountAdding           bool
	accountChoosing         bool
	accountLoading          bool
	accountAgent            string
	accountField            int
	accountAlias            textinput.Model
	accountName             textinput.Model
	accountSearch           textinput.Model
	accountSearching        bool
	accountService          resource.AccountServiceClient
	loginAccount            AccountLogin
	workflow                *accountWorkflow
	loginChoosing           bool
	loginAlias              string
	loginIndex              int
	focusApproval           bool
	approvalID              string
	approvalOffset          int
	interruptUntil          time.Time
	interruptKey            string
	approvalSent            map[string]bool
	permissionUpdating      map[string]bool
	localReports            map[string]string
	historyTimes            []int64
	historyPositions        []float64 // stable journal coordinates, not loaded-line offsets
	restartConfirm          *restartConfirmation
	questionDialog          *questionDialog
	questionSeen            map[string]bool
	restartBusy             bool
	lastPromptStart         int
	lastPromptEnd           int
	latestPrompt            string
	pulse                   int
	workingSince            int64
	backgroundSnapshots     map[string]backgroundSnapshot
	watchContext            context.Context
	backgroundLoading       map[string]bool
	backgroundErrors        map[string]string
	cursorOutput            *cursorWriter
	renderedResponses       map[*api.Event]renderedResponse
	toolActivities          map[toolActivityKey]cachedToolActivity
	renderedTools           map[toolRenderKey]string
	program                 *tea.Program
	pastes                  map[string]*pastedText
	pasteSelection          *chipSelection
	pasteDialog             *pasteDialog
}
type listing struct {
	projects       []*api.Project
	projectsErr    error
	projectsLoaded bool
	project        *api.Project
	sessions       []*api.Session
	err            error
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
	inputSession, inputRequest string
	text                       string
	err                        error
	sessionID                  string
}
type tick time.Time
type pulseTick struct{}

func pulseTimer() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return pulseTick{} })
}

type accountListing struct {
	request  uint64
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
	m.accountRequest++
	request := m.accountRequest
	service := m.accountClient()
	if service == nil {
		if c, ok := m.client.(*resourceclient.Client); ok {
			service = c.Accounts
		}
	}
	lifetime := m.contextFor(m.connectionRef())
	return func() tea.Msg {
		if service == nil {
			return accountListing{request: request, err: fmt.Errorf("Account service unavailable")}
		}
		ctx, cancel := context.WithTimeout(lifetime, 5*time.Second)
		defer cancel()
		var all []*resource.Account
		after := ""
		for {
			p, err := service.List(ctx, resource.AccountListRequest_builder{Size: 200, After: after}.Build())
			if err != nil {
				return accountListing{request: request, err: err}
			}
			all = append(all, p.GetItems()...)
			after = p.GetNext()
			if after == "" {
				return accountListing{request: request, accounts: all}
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
		check, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		s, err := c.Get(check, &api.SessionRef{Id: id})
		if err != nil {
			return err
		}
		id = s.Id
		project = &api.Project{Id: s.ProjectId, Name: s.ProjectName, Alias: s.ProjectAlias, Workspace: s.Workspace}
	}
	return RunProject(ctx, c, project, id, nil)
}

func RunProject(ctx context.Context, c api.SessionsClient, project *api.Project, id string, create ProjectCreator, login ...AccountLogin) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	input := newComposer()
	m := &model{ctx: ctx, client: c, input: input, view: viewport.New(80, 15), events: map[string][]*api.Event{}, cursor: map[string]uint64{}, width: 100, height: 30, wantID: id}
	m.initializeNavigation(project, id)
	m.createProjectSession = create
	if resources, ok := c.(*resourceclient.Client); ok {
		m.accountService = resources.Accounts
	}
	if len(login) > 0 {
		m.loginAccount = login[0]
	}
	m.debugRecorder = &debugRecorder{}
	m.cursorOutput = &cursorWriter{out: os.Stdout, keyboard: extendedKeyboard, recorder: m.debugRecorder}
	in := recordedKeyboardInput(os.Stdin, m.debugRecorder)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx), tea.WithMouseAllMotion(), tea.WithOutput(m.cursorOutput), tea.WithInput(in))
	m.program = p
	_, e := runKeyboardProgram(ctx, p, in, m.debugRecorder)
	if path, err := m.finishRecording(); err != nil {
		fmt.Fprintln(os.Stderr, "Could not save debug recording:", err)
	} else if path != "" {
		fmt.Fprintln(os.Stderr, "Debug recording saved:", path)
	}
	if m.redactDialog != nil {
		clear(m.redactDialog.body)
	}
	for _, r := range m.redactions {
		clear(r.body)
	}
	cancel()
	m.clearPathHints()
	if m.wisp != nil {
		m.wisp.Close()
	}
	for _, panel := range m.terminals {
		if panel.session != nil {
			panel.session.Close()
		}
	}
	if m.workflow != nil {
		m.workflow.close()
	}
	if m.watchCancel != nil {
		m.watchCancel()
	}
	return e
}
func (m *model) refresh() tea.Cmd {
	if m.resourceRefreshRunning {
		m.resourceRefreshAgain = true
		return nil
	}
	m.resourceRefreshRunning = true
	m.lastResourceRefresh = time.Now()
	panel := m.panelVisible() || m.panelFocus
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
		var projects []*api.Project
		var projectsErr error
		projectsLoaded := false
		if projectID != "" || panel {
			var ps *api.ProjectList
			var err error
			if c, ok := m.client.(interface {
				RegisteredProjects(context.Context) (*api.ProjectList, error)
			}); ok {
				ps, err = c.RegisteredProjects(ctx)
			} else {
				ps, err = m.client.Projects(ctx, &api.Empty{})
			}
			if err == nil {
				projectsLoaded = true
				projects = ps.Projects
				for _, candidate := range ps.Projects {
					if candidate.Id == projectID {
						p = candidate
						break
					}
				}
			} else {
				projectsErr = err
			}
		}
		return listing{sessions: v.Sessions, project: p, projects: projects, projectsErr: projectsErr, projectsLoaded: projectsLoaded}
	}
}
func timer() tea.Cmd { return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tick(t) }) }
func (m *model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), m.watchResources(), timer(), textarea.Blink, pulseTimer())
}
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
	if m.activityID == "" {
		m.activityID = core.ID()
	}
	activityID := m.activityID
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
	m.watchContext = ctx
	m.watchID = id
	m.watchEpoch++
	epoch := m.watchEpoch
	after := m.cursor[id]
	last := s.LastSeq
	agent, width := s.Agent, max(1, m.view.Width)
	if after == 0 && last > 0 {
		if m.historyOpening == nil {
			m.historyOpening = map[string]bool{}
		}
		m.historyOpening[id] = true
	}
	go func() {
		if after == 0 && last > 0 {
			start := uint64(0)
			if last > historyPageSize {
				start = last - historyPageSize
			}
			started := time.Now()
			batch, err := m.fetchHistory(ctx, id, start, "initial")
			if err != nil {
				if ctx.Err() == nil {
					m.program.Send(disconnected{id, err})
				}
				return
			}
			for _, e := range batch.Events {
				after = max(after, e.Seq)
			}
			if ctx.Err() != nil {
				return
			}
			page := m.prepareHistoryPage(ctx, historyPage{id: id, events: batch.Events, start: start, initial: true, epoch: epoch, started: started}, agent, width)
			if ctx.Err() != nil {
				return
			}
			m.program.Send(page)
		}
		// Replay the accumulated journal in pages, rendering once per page.
		// Only the selected conversation is subscribed; other agents keep running.
		for after < last {
			batch, err := m.fetchHistory(ctx, id, after, "catch_up")
			if err != nil {
				if ctx.Err() == nil {
					m.program.Send(disconnected{id, err})
				}
				return
			}
			before := after
			for _, e := range batch.Events {
				after = max(after, e.Seq)
			}
			if after == before {
				break
			}
			if ctx.Err() != nil {
				return
			}
			m.program.Send(caughtUp{id: id, events: batch.Events, epoch: epoch})
		}
		stream, e := m.client.Watch(ctx, &api.WatchRequest{SessionId: id, AfterSeq: after, ClientId: activityID})
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
	start := time.Now()
	defer func() {
		m.debugRecorder.Add(debugEvent{Kind: "transcript_render", Duration: time.Since(start).Microseconds(), Count: len(m.historyPositions)})
	}()
	m.promptSpans = nil
	m.codeButtons = nil
	copyBlocks := map[int][]codeButton{}
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
	conversationReady := false
	promptBlock := -1
	promptBlocks := map[int]string{}
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
	toolOutput := map[string]string{}
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
	events := m.transcriptEvents(s.Id)
	for _, e := range events {
		if e.Kind == "tool_call" && e.RequestId != "" && !question(e) && !m.hiddenEvents[e] {
			if activity, ok := m.cachedToolView(s.Agent, e); ok {
				toolCalls[e.RunId+"/"+e.RequestId] = activity
			} else {
				toolCalls[e.RunId+"/"+e.RequestId] = agentview.ToolActivity{Kind: "tool", Description: e.Text}
			}
		}
		if e.Kind == "tool_output" && e.RequestId != "" {
			key := e.RunId + "/" + e.RequestId
			toolOutput[key] = outputTail(toolOutput[key] + e.Text)
		}
		if e.Kind == "tool_result" && e.RequestId != "" {
			toolResults[e.RunId+"/"+e.RequestId] = e
		}
		if e.Kind == "approval_resolved" {
			decisions[e.RunId+"/"+e.RequestId] = e.Text
		}
	}
	for _, e := range events {
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
	for _, e := range events {
		if showHelp && !helped && e.Seq > helpAfter {
			add(m.localCommandView(s.Id), 0)
			helped = true
		}
		if e.Seq > 0 {
			sequence = e.Seq
		}
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
			key := e.RunId + "/" + e.RequestId
			_, pairedCall := toolCalls[key]
			if e.Kind == "tool_result" && pairedCall || e.Kind == "approval" && pairedApprovals[e] {
				continue
			}
			text := ""
			if e.Kind != "tool_call" || !pairedCall {
				text = eventViewCached(m, s, e, max(1, m.view.Width))
			}
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
						if final, ok := m.cachedToolView(s.Agent, result); ok {
							activity = final
						}
					}
					background := false
					if task, ok := backgroundTools[key]; ok {
						background = true
						state = task.Status
						if task.Active {
							state = "working"
						}
						if !task.Active && (state == "working" || state == "running") {
							state = "pending"
						}
					}
					text = indentBlock(m.cachedToolBody(s.Agent, e, activity, result, max(1, m.view.Width), state, background))
					if !background && activity.Kind == "command" && result == nil && e.RunId == s.RunId && m.activeWork() && toolOutput[key] != "" {
						text += "\n" + liveOutputView(toolOutput[key], m.view.Width)
					}
				}
			}
			if e.Kind == "approval" {
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
				if e.Seq > 0 {
					switch e.Kind {
					case "input", "assistant":
						conversationReady = conversationReady || strings.TrimSpace(e.Text) != ""
					case "tool_call", "tool_result", "approval":
						conversationReady = true
					}
				}
				if m.isPendingInput(e) {
					text = muted.Render(ansi.Strip(text))
				}
				add(text, e.TimeMs)
				if e.Kind == "input" {
					promptBlock = len(lines) - 1
					m.latestPrompt = e.Text
					promptBlocks[promptBlock] = e.Text
				}
				if e.Kind == "assistant" {
					replyIndex = len(lines) - 1
					copyBlocks[replyIndex] = m.renderedResponses[e].buttons
				}
			}
		}
		if hint := authHint(s, e); hint != "" {
			add(indentBlock(warning.Render(ansi.Hardwrap(safeText(hint), max(1, m.view.Width-2), true))), e.TimeMs)
		}
	}
	if start, loaded := m.historyStart[s.Id]; loaded && m.historyOpening[s.Id] && (conversationReady || start == 0) {
		delete(m.historyOpening, s.Id)
		follow = true
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
		for _, button := range copyBlocks[i] {
			button.y += start
			m.codeButtons = append(m.codeButtons, button)
		}
		rows := strings.Split(block, "\n")
		for row := range rows {
			m.historyTimes = append(m.historyTimes, times[i])
			m.historyPositions = append(m.historyPositions, float64(sequences[i])+float64(row)/float64(len(rows)))
		}
		if i == promptBlock {
			m.lastPromptStart = start
			m.lastPromptEnd = len(m.historyTimes)
		}
		if text, ok := promptBlocks[i]; ok {
			m.promptSpans = append(m.promptSpans, promptSpan{start: start, end: len(m.historyTimes), text: text})
		}
	}
	m.view.SetContent(strings.Join(lines, "\n\n"))
	if m.selectingTools() {
		for _, t := range m.toolTargets() {
			if t.seq == m.toolSelector.seq {
				m.revealTool(t.row)
				break
			}
		}
	} else if follow {
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
		return m.replyApproval(m.selectedApproval(), kind != "deny", text)
	}
	s := m.current()
	if s == nil {
		return nil
	}
	id, run := s.Id, s.RunId
	clientID := core.ID()
	if kind == "send" {
		m.queueInput(id, run, clientID, text)
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		ctrl := &api.Control{SessionId: id, RunId: run, ClientId: clientID}
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
		r := result{text: message, err: e}
		if kind == "send" {
			r.inputSession, r.inputRequest = id, clientID
		}
		return r
	}
}
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer m.debugUpdate(msg)()
	if tick, ok := msg.(performanceTick); ok {
		return m, m.performanceUpdate(tick)
	}
	if r, ok := msg.(recordingSaved); ok {
		m.receiveRecording(r)
		return m, nil
	}
	if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeyF9 && !k.Paste {
		return m, m.toggleRecording()
	}
	defer m.rememberNotice()
	if v, ok := msg.(memoryTargets); ok {
		m.receiveMemoryTargets(v)
		return m, nil
	}
	if v, ok := msg.(memoryCopied); ok {
		m.receiveMemoryCopied(v)
		return m, nil
	}
	if v, ok := msg.(memoryResult); ok {
		m.receiveMemory(v)
		return m, nil
	}
	if v, ok := msg.(settingsResult); ok {
		m.receiveSettings(v)
		return m, nil
	}
	if v, ok := msg.(logsResult); ok {
		if m.report == v.report {
			m.report.text = v.text
		}
		return m, nil
	}
	if v, ok := msg.(redactSent); ok {
		if v.err != nil {
			m.removePendingInput(v.id, v.request)
			m.render()
		}
		for _, token := range v.tokens {
			if r := m.redactions[token]; r != nil {
				clear(r.body)
				delete(m.redactions, token)
			}
		}
		m.redactSending = false
		m.pruneRedactions()
		if v.err != nil {
			m.notice = v.err.Error() + "; reenter the secret with @redact"
		} else {
			m.notice = "send · accepted (secret files swept after 8 hours idle)"
		}
		return m, nil
	}
	var activity tea.Cmd
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
		// Report the first input immediately after a pause; steady activity is
		// covered by the heartbeat. Capture the old session before navigation.
		if time.Since(m.lastUIInput) >= time.Second {
			m.lastActivityReport = time.Time{}
			m.lastUIInput = time.Now()
			activity = m.reportActivity()
		}
	}
	next, cmd := m.update(msg)
	if k, ok := msg.(tea.KeyMsg); ok && k.Paste {
		if token := m.inlineContext(); token != nil {
			m.inlineDismissed = token.signature
		}
	}
	m.pruneRedactions()
	if _, ok := msg.(listing); ok {
		cmd = tea.Batch(cmd, m.watchResources())
	}
	if m.resourceRefreshAgain && !m.resourceRefreshRunning {
		m.resourceRefreshAgain = false
		cmd = tea.Batch(cmd, m.refresh())
	}
	return next, tea.Batch(activity, cmd, m.syncPathHints())
}

func (m *model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && !k.Paste && !m.terminalFocused() {
		switch k.String() {
		case "ctrl+d":
			return m, tea.Quit
		case "ctrl+c":
			m.copyFocusedText()
			return m, nil
		case "esc":
			if m.previewInteraction() && m.selectionValid() && m.textSelection.text() != "" {
				m.textSelection = nil
				return m, nil
			}
		}
	}

	switch v := msg.(type) {
	case tea.KeyMsg, tea.WindowSizeMsg, tea.BlurMsg:
		m.panelHoverY = 0
		m.codeHover = nil
		if m.filePreview != nil {
			m.filePreview.hover = ""
		}
	case tea.MouseMsg:
		m.codeHover = nil
		if m.filePreview != nil {
			m.filePreview.hover = ""
		}
		if m.selectionMouse(v) {
			return m, nil
		}
		if handled, cmd := m.panelMouse(v); handled {
			return m, cmd
		}
		m.focusConversationMouse(v)
	}
	if m.memoryPage != nil {
		switch v := msg.(type) {
		case tea.KeyMsg:
			return m, m.memoryKey(v)
		case tea.MouseMsg:
			return m, m.memoryMouse(v)
		}
	}
	if m.settingsPage != nil {
		switch v := msg.(type) {
		case tea.KeyMsg:
			return m, m.settingsKey(v)
		case tea.MouseMsg:
			return m, m.settingsMouse(v)
		}
	}
	if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeyCtrlP && !k.Paste && m.workflow == nil && m.redactDialog == nil && m.questionDialog == nil && m.pasteDialog == nil && m.restartConfirm == nil {
		return m, m.openSettings()
	}

	switch v := msg.(type) {
	case terminalOpened:
		if p := m.terminals[v.id]; p != nil {
			p.starting = false
			p.session = v.session
			p.err = v.err
			m.closeSuccessfulTerminal(v.id)
			m.resize()
		} else if v.session != nil {
			v.session.Close()
		}
		return m, nil
	case terminalChanged:
		m.closeSuccessfulTerminal(v.id)
		return m, nil
	case pathHintDue:
		return m, m.fetchPathHints(v)
	case pathHintResult:
		m.receivePathHints(v)
		return m, nil
	case tea.KeyMsg:
		if m.redactDialog != nil {
			return m, m.redactKey(v)
		}
		if v.Type == tea.KeyF20 && !v.Paste {
			return m, m.toggleTerminal()
		}
		if m.terminalFocused() {
			m.terminalKey(v)
			return m, nil
		}
	case tea.MouseMsg:
		if m.terminalMouse(v) {
			return m, nil
		}
	}
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
	case pasteSent:
		if v.result.err != nil {
			if s := m.current(); s != nil && s.Id == v.id && m.input.Value() == "" {
				m.input.SetValue(v.draft)
			} else {
				if m.drafts == nil {
					m.drafts = map[string]string{}
				}
				if m.drafts[v.id] == "" {
					m.drafts[v.id] = v.draft
				}
			}
		}
		return m.Update(v.result)
	case pasteUploaded:
		v.dialog.busy = false
		if v.dialog.direct {
			p := m.pastes[v.token]
			if p == nil || p.upload != v.dialog {
				return m, nil
			}
			p.upload = nil
			s := m.current()
			if s == nil || s.Id != v.dialog.session || s.RunId != v.dialog.run {
				return m, nil
			}
			text := m.input.Value()
			if d := v.dialog.question; d != nil {
				if m.questionDialog != d || d.page != v.dialog.page {
					return m, nil
				}
				text = d.other[d.page].Value()
			}
			if !strings.Contains(text, v.token) {
				return m, nil
			}
			if v.err != nil {
				m.notice = "Upload failed; original text retained: " + v.err.Error()
				return m, nil
			}
			if v.path == "" {
				m.notice = "Upload returned no path; original text retained."
				return m, nil
			}
			p.path, p.file = v.path, true
			m.notice = "File stored. Only its path will be sent; t restores full text."
			return m, nil
		}
		if v.err != nil {
			v.dialog.message = "Upload failed; original text retained: " + v.err.Error()
			return m, nil
		}
		if v.path == "" {
			v.dialog.message = "Upload returned no path; original text retained."
			return m, nil
		}
		if p := m.pastes[v.token]; p != nil {
			p.path = v.path
			if m.pasteDialog == v.dialog {
				p.file = true
			}
		}
		v.dialog.message = "File stored. Only its path will be sent; t restores full text."
		return m, nil
	case workflowOutput:
		if m.workflow == v.flow {
			m.workflow.output += v.text
			if len(m.workflow.output) > 32768 {
				m.workflow.output = m.workflow.output[len(m.workflow.output)-32768:]
			}
			return m, m.workflow.wait()
		}
		return m, nil
	case workflowWritten:
		if m.workflow == v.flow {
			m.workflow.sending = false
			if v.err != nil {
				m.workflow.message = "Code could not be submitted."
			} else {
				m.workflow.message = "Code submitted; waiting for provider…"
			}
		}
		return m, nil
	case workflowDone:
		if m.workflow != v.flow {
			return m, nil
		}
		m.workflow.close()
		m.workflow = nil
		if v.flow.create {
			r := result{text: "session created", err: v.err}
			if v.session != nil && v.err == nil {
				r.sessionID = v.session.Id
			}
			return m.Update(r)
		}
		return m.Update(accountLoggedIn{v.err})
	case contextSent:
		if c := m.contextCapture; c != nil && c.request == v.request && v.err != nil {
			c.report.text = "Context unavailable: " + v.err.Error()
			m.contextCapture = nil
		}
		return m, nil
	case modelCatalogLoaded:
		return m, m.acceptModelCatalog(v)
	case historyPage:
		if !m.applyHistoryPage(v) {
			return m, nil
		}
		background := m.loadBackgroundHistory(v)
		if background != nil {
			if m.backgroundLoading == nil {
				m.backgroundLoading = map[string]bool{}
			}
			m.backgroundLoading[v.id] = true
		}
		if v.err == nil && m.current() != nil && m.current().Id == v.id {
			// Find conversation content first, then keep a viewport-sized runway.
			return m, tea.Batch(m.requestOlderHistory(v.initial), background)
		}
		return m, background
	case backgroundHistory:
		if v.epoch != 0 && v.epoch != m.watchEpoch {
			return m, nil
		}
		if m.backgroundSnapshots == nil {
			m.backgroundSnapshots = map[string]backgroundSnapshot{}
		}
		if m.backgroundErrors == nil {
			m.backgroundErrors = map[string]string{}
		}
		if v.err == nil {
			m.backgroundSnapshots[v.id] = v.snapshot
		}
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
	case permissionResult:
		delete(m.permissionUpdating, v.id)
		if current := m.current(); current != nil && current.Id == v.id && current.RunId == v.run {
			if v.err != nil {
				m.notice = "Permission update failed; refresh to check the saved policy: " + v.err.Error()
				if status.Code(v.err) == codes.Unimplemented {
					m.notice = "Permission RPC unavailable: update cxz on the host, run cxz install --recreate, then cxz project recreate WORKSPACE (replaces container; writable layer lost). See docs/cli.md.\nRPC error: " + v.err.Error()
				}
			} else {
				m.notice = "Permission " + v.mode + " saved for this session"
			}
		}
		return m, m.refresh()
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
			m.notice = "Approval decision sent"
		}
		if m.focusApproval {
			m.focusApproval = false
			m.input.Focus()
		}
		m.resize()
		m.render()
		return m, m.refresh()
	case tea.MouseMsg:
		if m.filePreviewMouse(v) {
			return m, nil
		}
		m.lastUIInput = time.Now()
		if v.X < m.contentOffset() || v.X >= m.contentOffset()+m.width || m.panelFocus {
			return m, nil
		}
		v.X -= m.contentOffset()
		if m.composerStatusMouse(v) {
			return m, nil
		}
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
		if m.modelPicker != nil || m.pasteDialog != nil || m.redactDialog != nil {
			return m, nil
		}
		if m.focusApproval && v.Y >= m.view.Height && v.Y < m.height-m.input.Height()-3-m.terminalHeight() {
			switch v.Button {
			case tea.MouseButtonWheelUp:
				m.approvalOffset = max(0, m.approvalOffset-3)
			case tea.MouseButtonWheelDown:
				m.approvalOffset += 3
			}
			return m, nil
		}
		if !m.projectView && !m.accountView && v.Y < m.view.Height {
			if m.codeBlockMouse(v) {
				return m, nil
			}
			if v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress && m.openFilePreview(v.Y) {
				return m, nil
			}
			m.beginSelection(v)
			m.view, _ = m.view.Update(v)
			if v.Button == tea.MouseButtonWheelUp || v.Button == tea.MouseButtonWheelDown {
				return m, m.loadOlderHistory()
			}
			return m, nil
		}
		return m, nil
	case renameResult:
		m.renameBusy = false
		if v.err != nil {
			m.notice = v.err.Error()
			return m, nil
		}
		for _, s := range append(append([]*api.Session(nil), m.sessions...), m.allSessions...) {
			if s.Id == v.id {
				s.Alias = v.alias
			}
		}
		m.renaming = false
		m.aliasInput.Blur()
		m.focusList = false
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
		if v.request != m.accountRequest {
			return m, nil
		}
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
		m.terminalWidth = v.Width
		m.width = min(v.Width, maxViewWidth)
		m.height = v.Height

		m.resize()
		m.render()
	case tick:
		m.watch()
		var history tea.Cmd
		if m.historyOpening[m.watchID] {
			// Resume an unfinished opening when returning to a session whose
			// previous page arrived while another conversation was selected.
			history = m.loadOlderHistory()
		}
		return m, tea.Batch(timer(), m.periodicRefresh(), m.reportActivity(), m.pollSettings(), history)
	case resourcesChanged:
		if v.generation != m.resourceWatchGeneration {
			return m, nil
		}
		if time.Since(m.resourceWatchStarted) > 10*time.Second {
			m.resourceRetryDelay = 0
		}
		if m.resourceRefreshPending {
			return m, nil
		}
		m.resourceRefreshPending = true
		return m, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return resourcesRefreshDue{} })
	case resourcesRefreshDue:
		m.resourceRefreshPending = false
		return m, m.refresh()
	case resourcesWatchEnded:
		if v.generation != m.resourceWatchGeneration {
			return m, nil
		}
		m.resourcesWatching = false
		if m.ctx == nil || m.ctx.Err() != nil {
			return m, nil
		}
		m.resourceRetryDelay = min(30*time.Second, max(time.Second, m.resourceRetryDelay*2))
		return m, tea.Tick(m.resourceRetryDelay, func(time.Time) tea.Msg { return resourcesWatchRetry{} })
	case resourcesWatchRetry:
		return m, m.watchResources()
	case activityReported:
		return m, nil
	case listing:
		m.resourceRefreshRunning = false
		if v.err != nil {
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
		m.updatePanel(v)
		if v.project != nil && (m.project == nil || m.project.Id == v.project.Id) {
			m.project = v.project
		}
		if m.wantID != "" {
			for _, s := range v.sessions {
				if s.Id == m.wantID {
					m.project = &api.Project{Id: s.ProjectId, Name: s.ProjectName, Alias: s.ProjectAlias, Workspace: s.Workspace}
					for _, p := range m.panelProjects {
						if p.Id == s.ProjectId {
							m.project = p
							break
						}
					}
					break
				}
			}
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
		return m, nil
	case received:
		m.receiveEvent(v, true)
	case caughtUp:
		if v.epoch != m.watchEpoch {
			return m, nil
		}
		for _, e := range v.events {
			m.receiveEvent(received{v.id, e}, false)
		}
		if current := m.current(); current != nil && current.Id == v.id {
			m.render()
		}
		return m, nil
	case disconnected:
		if c := m.contextCapture; c != nil && c.id == v.id {
			c.report.text = "Disconnected while querying context. Reopen /context after reconnecting."
			m.contextCapture = nil
		}
		if m.watchID == v.id {
			m.watchID = ""
			delete(m.historyOpening, v.id)
			m.notice = "event connection lost; reconnecting from saved cursor"
		}
	case restartFinished:
		m.restartBusy = false
		if v.err != nil {
			m.notice = v.err.Error()
		} else {
			m.notice = "Agent restarted · same session · saved permission policy"
		}
		return m, m.refresh()
	case result:
		if v.err != nil && v.inputRequest != "" {
			m.removePendingInput(v.inputSession, v.inputRequest)
			m.render()
		}
		m.busy = false
		if v.err != nil {
			m.notice = v.err.Error()
		} else {
			m.notice = v.text
			if v.sessionID != "" {
				m.wantID = v.sessionID
				m.panelFocus = false
				m.projectView = false
				m.accountView = false
				return m, tea.Batch(m.input.Focus(), m.refresh())
			}
		}
		return m, m.refresh()
	case tea.KeyMsg:
		m.lastUIInput = time.Now()
		if m.renaming {
			return m, m.renameKey(v)
		}
		if m.panelFocus && !m.accountView && m.workflow == nil && !m.creating {
			return m, m.panelKey(v)
		}
		if m.previewInteraction() {
			if m.previewVisible() && m.filePreview.focused {
				return m, m.filePreviewKey(v)
			}
			if m.selectingTools() {
				return m, m.toolSelectorKey(v)
			}
		}
		if m.pasteDialog != nil {
			return m, m.pasteKey(v)
		}
		if handled, cmd := m.chipKey(v); handled {
			m.resize()
			return m, cmd
		}
		if m.capturePaste(v) {
			return m, nil
		}
		if m.workflow != nil {
			return m, m.workflowKey(v)
		}
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
			if m.deletePathWord(v) {
				return m, nil
			}
			if handled, cmd := m.pathHintKey(v); handled {
				return m, cmd
			}
			if handled, cmd := m.commandKey(v); handled {
				return m, cmd
			}
			if m.inlineKey(v) {
				return m, nil
			}
		}
		if v.String() == "ctrl+q" {
			m.focusPanel()
			return m, m.refresh()
		}
		if m.projectView && !m.creating {
			return m, m.panelKey(v)
		}
		if m.focusApproval && v.String() != "tab" && v.String() != "shift+tab" && v.String() != "ctrl+d" {
			return m, m.approvalKey(v)
		}
		switch v.String() {
		case "ctrl+d":
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
			if m.project != nil && m.project.State != "connection" {
				m.backToProject()
				return m, m.projectAction(v)
			}
			m.creationConnection = m.connectionRef()
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
				before := m.input.Value()
				m.input.InsertString("\n")
				if partialPasteEdit(before, m.input.Value(), m.pastes) {
					m.input.SetValue(before)
				}
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
			if text == "/redact" {
				m.input.Reset()
				m.notice = "Use @redact inside your message, then Enter"
				return m, nil
			}
			if strings.HasPrefix(text, "/redact ") || strings.HasPrefix(text, "/redact\n") || strings.HasPrefix(text, "/redact\t") {
				m.input.Reset()
				m.notice = "Use @redact inside your message; enter the secret only in its dialog"
				return m, nil
			}
			if m.hasRedactions(text) {
				return m, m.sendRedactions(text)
			}
			if text == "/paste" {
				return m, m.openPastes()
			}
			draft := text
			text = expandPastes(text, m.pastes)
			if draft != text && !m.creating {
				m.input.Reset()
				return m, m.sendPastes(draft, text)
			}
			if m.creating && m.project != nil && m.project.Workspace != "" {
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
				lifetime := m.contextFor(m.creationConnection)
				return m, func() tea.Msg {
					path, e := workspacePath(lifetime, text)
					if e != nil {
						return result{err: e}
					}
					ctx, cancel := context.WithTimeout(lifetime, 30*time.Minute)
					defer cancel()
					if os.Getenv("CXZ_PROJECT_ID") != "" && !transport.IsRemote(lifetime) {
						s, e := m.client.Create(ctx, &api.CreateRequest{Workspace: path, Agent: kind, Model: settings.From(lifetime).Model(kind), ClientId: core.ID(), Account: account})
						if e != nil {
							return result{err: e}
						}
						return result{text: "session created", sessionID: s.Id}
					}
					if !transport.IsRemote(lifetime) {
						path, e = dockerx.EnginePath(path)
					}
					if e != nil {
						return result{err: e}
					}
					s, e := m.client.Open(ctx, &api.ProjectRequest{Workspace: path, Agent: kind, Model: settings.From(lifetime).Model(kind), NewSession: true, ClientId: core.ID(), Account: account})
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
			if localName == "/terminal" {
				return m, m.toggleTerminal()
			}
			if localName == "/view" {
				return m, m.viewCommand(text)
			}
			if localName == "/record" {
				return m, m.recordCommand(text)
			}
			if localName == "/memory" {
				return m, m.openMemory(m.current())
			}
			if localName == "/settings" {
				return m, m.openSettings()
			}
			if localName == "/logs" {
				return m, m.logsCommand(text)
			}
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
			return m, m.sendPastes(draft, text)
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
		before := m.input.Value()
		m.input, cmd = m.input.Update(msg)
		if _, ok := msg.(tea.KeyMsg); ok {
			m.snapChipCursor()
		}
		if partialPasteEdit(before, m.input.Value(), m.pastes) {
			m.input.SetValue(before)
			m.notice = "Paste chips are indivisible; Ctrl+P to preview or delete."
		}
	}
	m.resize()
	if k, ok := msg.(tea.KeyMsg); ok && (k.String() == "pgup" || k.String() == "pgdown") {
		m.view, _ = m.view.Update(msg)
	}
	return m, cmd
}
func (m *model) View() (out string) {
	start := time.Now()
	defer func() {
		out = m.recordingBadge(out)
		m.debugRecorder.Add(debugEvent{Kind: "render", Duration: time.Since(start).Microseconds(), Count: len(out)})
	}()
	if m.memoryPage != nil {
		return m.memoryScreen()
	}
	if m.settingsPage != nil {
		return m.settingsScreen()
	}
	defer func() { out = m.wideScreen(out) }()
	m.anchorCursor()
	if m.workflow != nil {
		return m.workflowScreen()
	}
	if m.width > 0 && (m.width < 40 || m.height < 14) {
		return screen("cxz\nResize terminal to 40 × 14 or larger.\nCtrl+D detach", m.width, m.height)
	}
	if (m.panelFocus || m.projectView) && !m.accountView && !m.creating && !m.panelVisible() {
		return m.panelScreen()
	}
	if m.accountView {
		return m.accountScreen()
	}
	if m.projectView && !m.creating {
		body := indentBlock("Select a session from Projects.\n\nn new session · a accounts")
		if notice := m.navigationNotice(); notice != "" {
			body += "\n\n" + indentBlock(warning.Render(ansi.Hardwrap(safeText(notice), max(1, m.width-4), true)))
		}
		return screen(body, m.width, m.height)
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

func (m *model) receiveEvent(v received, repaint bool) {
	if v.event.Kind == "input" {
		m.removePendingInput(v.id, v.event.RequestId)
	}
	if v.event.Seq > m.cursor[v.id] {
		m.captureContext(v.id, v.event)
		if v.event.Kind == "permission" && (v.event.Text == "ask" || v.event.Text == "full") {
			for _, session := range m.sessions {
				if session.Id == v.id && session.RunId == v.event.RunId && v.event.Seq >= session.LastSeq {
					session.PermissionMode = v.event.Text
				}
			}
		}
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
		if s := m.current(); s != nil && s.Id == v.id && repaint && v.event.Kind != "raw" {
			m.render()
		}
	}
}
