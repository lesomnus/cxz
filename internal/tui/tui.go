package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/bed"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"github.com/lesomnus/cxz/internal/notification"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/versionpin"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type model struct {
	elicitationRequest       string
	elicitationChoice        int
	auxiliarySummaries       map[string][]auxiliary.Summary
	auxiliaryJobs            map[string]*auxiliary.Job
	auxiliaryConfigs         map[string]auxiliary.SessionConfig
	auxiliaryVersions        map[string]uint64
	auxiliaryPending         map[string]bool
	auxiliaryLoadingRows     map[int]bool
	auxiliaryPolling         bool
	auxiliaryChecked         time.Time
	auxiliaryError           string
	library                  *libraryPage
	seedMemory               string
	download                 *fileDownload
	bottomButtonHover        bool
	sessionNavigation        sessionNavigation
	pinGeneration            string
	pinChecked               time.Time
	pinRestart               *versionpin.Restart
	autoStarted, autoChecked time.Time
	autoChecking             bool
	autoState                cxzupdate.State
	autoCandidate            string
	autoRestart              *cxzupdate.Restart
	autoRestorePosition      float64

	alertPlayer             *notification.Player
	liveRenderWidth         *atomic.Int64
	renderedInputs          map[inputRenderKey]string
	renderedSummaries       map[summaryRenderKey]string
	contextStatusCache      *contextStatusCache
	errorDialog             *errorDialog
	sessionActivity         map[string]*sessionActivity
	sessionBackground       map[string]*sessionBackgroundCheck
	backgroundStateCache    map[string]backgroundStateCache
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
	toolHover               *toolSelector
	toolClick               *toolClick
	toolRows                map[int]uint64
	noticeLogs              []noticeLog
	lastLoggedNotice        string
	filePreview             *filePreview
	pendingInputs           map[string][]*api.Event
	promptSpans             []promptSpan
	inlineDismissed         string
	mentionDismissed        string
	mentionSignature        string
	mentionSelected         int
	redactDialog            *redactDialog
	redactions              map[string]*redaction
	redactSending           bool
	redactStore             secretFiles
	pathHints               *pathHints
	hostFileCheck           *hostFileCheck
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
	resumePending           map[string]bool
	panelHintHover          string // Footer hint under the pointer, by its key.
	quotaParses             map[quotaParseKey]quotaParse
	panelProjects           []*api.Project
	allSessions             []*api.Session
	panelError              string
	projectView             bool
	deletingID              string
	deleteConfirm           *sessionDeleteConfirmation
	busy                    bool
	createProjectSession    ProjectCreator
	ctx                     context.Context
	client                  api.SessionsClient
	sessions                []*api.Session
	selected                int
	input                   bed.Model
	drafts                  map[string]string
	localHelp               map[string]uint64
	localOutput             map[string]string
	hintSelected            int
	hintOffset              int
	hintDismissed           bool
	renaming                bool
	renameBusy              bool
	renameProject           bool
	renameID                string
	aliasInput              textinput.Model
	usageReports            map[string]string
	usageGeneration         map[string]uint64
	accountQuotas           map[string][]agentview.Window
	quotaWindows            []agentview.Window
	quotaState              string
	modelPicker             *modelPicker
	modelPickerEpoch        uint64
	// modelCatalogs caches one capability record per run, so /model and /effort
	// are one lookup rather than two, and a live `models` event replaces it.
	modelCatalogs       map[string]*modelCatalog
	settingsPage        *settingsPage
	sessionArchive      *sessionArchive
	memoryPage          *memoryPage
	report              *reportOverlay
	contextCapture      *contextCapture
	hiddenEvents        map[*api.Event]bool
	view                viewport.Model
	focusList, creating bool
	notice              string
	width, height       int
	events              map[string][]*api.Event
	cursor              map[string]uint64
	historyWindows      map[string]*historyWindow
	windowPolicy        *historypolicy.Window
	historyLoading      map[string]uint64 // End sequence of each in-flight older page.
	historyStart        map[string]uint64
	historyOpening      map[string]bool // Keep loading through pages containing only bookkeeping events.
	historyShimmer      *historyShimmer
	watchCancel         context.CancelFunc
	watchID             string
	watchEpoch          uint64
	watchSequence       uint64
	sessionWatches      map[string]*sessionWatch
	watchBootstrap      chan struct{}
	wantID              string
	accounts            []*resource.Account
	accountIndex        int
	accountView         bool
	accountAdding       bool
	accountChoosing     bool
	accountLoading      bool
	accountAgent        string
	accountField        int
	accountAlias        textinput.Model
	accountName         textinput.Model
	accountSearch       textinput.Model
	accountSearching    bool
	accountService      resource.AccountServiceClient
	loginAccount        AccountLogin
	workflow            *accountWorkflow
	loginChoosing       bool
	loginAlias          string
	loginIndex          int
	focusApproval       bool
	approvalID          string
	approvalOffset      int
	interruptUntil      time.Time
	interruptKey        string
	approvalSent        map[string]bool
	permissionUpdating  map[string]bool
	localReports        map[string]string
	historyTimes        []int64
	historyPositions    []float64 // stable journal coordinates, not loaded-line offsets
	workingToolRows     map[int]bool
	restartConfirm      *restartConfirmation
	questionDialog      *questionDialog
	questionDrafts      map[string]*questionDialog
	questionSeen        map[string]bool
	restartBusy         bool
	lastPromptStart     int
	lastPromptEnd       int
	latestPrompt        string
	pulse               int
	blinkFrom           int
	composerBlink       bool // enabled when Init starts the event loop
	composerFocusCmd    tea.Cmd
	blurred             bool
	workingSince        int64
	backgroundSnapshots map[string]backgroundSnapshot
	watchContext        context.Context
	backgroundLoading   map[string]bool
	backgroundErrors    map[string]string
	cursorOutput        *cursorWriter
	renderedResponses   map[*api.Event]renderedResponse
	toolActivities      map[toolActivityKey]cachedToolActivity
	renderedTools       map[toolRenderKey]string
	program             *tea.Program
	pastes              map[string]*pastedText
	pasteSelection      *chipSelection
	pasteDialog         *pasteDialog
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
	resumeSession              string
	inputSession, inputRequest string
	inputText                  string
	status                     string
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
	if c, ok := versionpin.ClientFrom(ctx); ok {
		pin, _ := versionpin.Load(c.Root)
		if pin.Ready {
			m.pinGeneration = pin.Generation
		}
	}
	m.autoStarted = time.Now()
	var restore cxzupdate.Resume
	if frontend, ok := cxzupdate.ClientFrom(ctx); ok {
		restore, _ = cxzupdate.TakeResume(frontend.Root)
		if restore.Session != "" {
			id = restore.Session
			m.wantID = id
			m.autoRestorePosition = restore.Position
		}
	}
	m.initializeNavigation(project, id)
	if restore.Connection != "" {
		m.panelWantConnection = restore.Connection
	}
	if restore.Project != "" && restore.Session == "" {
		m.panelWantKey = "p:" + restore.Project
	}
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
	// Focus reporting is what lets the bright green mean the keyboard: without
	// it a window left in the background still claims to hold it.
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx), tea.WithMouseAllMotion(), tea.WithReportFocus(), tea.WithOutput(m.cursorOutput), tea.WithInput(in))
	m.program = p
	_, e := runKeyboardProgram(ctx, p, in, m.cursorOutput, m.debugRecorder)
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
	if e == nil && m.pinRestart != nil {
		return m.pinRestart
	}
	if e == nil && m.autoRestart != nil {
		return m.autoRestart
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
	m.composerBlink = true
	m.input.Cursor.SetMode(cursor.CursorBlink)
	var blink tea.Cmd
	if m.input.Focused() {
		blink = m.input.Focus()
	}
	return tea.Batch(m.refresh(), m.watchResources(), timer(), pulseTimer(), blink)
}
func (m *model) current() *api.Session {
	if len(m.sessions) == 0 {
		return nil
	}
	m.selected = min(m.selected, len(m.sessions)-1)
	return m.sessions[m.selected]
}

// The project list carries sessions the current project does not, so a lookup
// by ID has to consult both.
func (m *model) session(id string) *api.Session {
	for _, list := range [][]*api.Session{m.sessions, m.allSessions} {
		for _, s := range list {
			if s.Id == id {
				return s
			}
		}
	}
	return nil
}
func (m *model) startSessionWatch(s *api.Session) {
	id := s.Id
	ctx, cancel := context.WithCancel(m.ctx)
	m.watchSequence++
	epoch := m.watchSequence
	m.sessionWatches[id] = &sessionWatch{ctx: ctx, cancel: cancel, epoch: epoch, active: true}
	activityID := m.activityID
	after := m.cursor[id]
	last := s.LastSeq
	agent, width := s.Agent, max(1, m.view.Width)
	if m.liveRenderWidth == nil {
		m.liveRenderWidth = &atomic.Int64{}
		m.liveRenderWidth.Store(int64(width))
	}
	renderWidth := m.liveRenderWidth
	if after == 0 && last > 0 {
		if m.historyOpening == nil {
			m.historyOpening = map[string]bool{}
		}
		m.historyOpening[id] = true
	}
	go func() {
		select {
		case m.watchBootstrap <- struct{}{}:
		case <-ctx.Done():
			return
		}
		bootstrapping := true
		defer func() {
			if bootstrapping {
				<-m.watchBootstrap
			}
		}()
		if after == 0 && last > 0 {
			start := uint64(0)
			if last > historyPageSize {
				start = last - historyPageSize
			}
			started := time.Now()
			batch, err := m.fetchHistory(ctx, id, start, "initial")
			for err == nil && start > 0 && historyBatchFloor(batch.Events) == 0 && !hasConversationEvents(batch.Events) {
				end := start
				start = 0
				if end > historyPageSize {
					start = end - historyPageSize
				}
				var older *api.EventBatch
				older, err = m.fetchHistory(ctx, id, start, "initial_older")
				if err == nil {
					var prefix []*api.Event
					for _, event := range older.Events {
						if event.Seq <= end {
							prefix = append(prefix, event)
						}
					}
					batch.Events = append(prefix, batch.Events...)
				}
			}
			if err != nil {
				if ctx.Err() == nil {
					m.program.Send(sessionWatchEnded{id: id, epoch: epoch, err: err})
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
		// Every listed session keeps its own cursor and subscription.
		for after < last {
			batch, err := m.fetchHistory(ctx, id, after, "catch_up")
			if err != nil {
				if ctx.Err() == nil {
					m.program.Send(sessionWatchEnded{id: id, epoch: epoch, err: err})
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
			page := m.prepareHistoryPage(ctx, historyPage{events: batch.Events}, agent, int(renderWidth.Load()))
			if ctx.Err() != nil {
				return
			}
			m.program.Send(caughtUp{id: id, events: batch.Events, epoch: epoch, prepared: &page})
		}
		<-m.watchBootstrap
		bootstrapping = false
		stream, e := m.client.Watch(ctx, &api.WatchRequest{SessionId: id, AfterSeq: after, ClientId: activityID})
		if e == nil {
			e = collectLiveEvents(ctx, stream, func(events []*api.Event) {
				start := time.Now()
				page := m.prepareHistoryPage(ctx, historyPage{events: events}, agent, int(renderWidth.Load()))
				m.debugRecorder.Add(debugEvent{Kind: "live_batch", Duration: time.Since(start).Microseconds(), Count: len(events)})
				if ctx.Err() == nil {
					m.program.Send(caughtUp{id: id, events: events, epoch: epoch, prepared: &page})
				}
			})
		}
		if ctx.Err() == nil {
			m.program.Send(sessionWatchEnded{id: id, epoch: epoch, err: e})
		}
	}()
}
func (m *model) render() {
	start := time.Now()
	defer func() {
		// The row count is what survived the window; the cost is what render walked
		// to produce it, which is every loaded event. Without both, a rebuild that
		// is slow cannot be told from one that is merely large. Summing is itself
		// proportional to the events, so only do it while a recording runs.
		e := debugEvent{Kind: "transcript_render", Duration: time.Since(start).Microseconds(), Count: len(m.historyPositions)}
		if s := m.current(); s != nil && m.debugRecorder.recording() {
			for _, loaded := range m.events[s.Id] {
				e.Events++
				e.Bytes += len(loaded.Text) + len(loaded.Payload)
			}
			// The window trims on turn boundaries and never below one turn, so a
			// single enormous turn keeps everything however large it grows.
			e.Turns = m.historyWindow(s.Id).measuredTurns
		}
		m.debugRecorder.Add(e)
	}()
	m.promptSpans = nil
	m.codeButtons = nil
	m.workingToolRows = nil
	m.auxiliaryLoadingRows = nil
	m.toolRows = map[int]uint64{}
	copyBlocks := map[int][]codeButton{}
	toolBlocks := map[int]bool{}
	inspectBlocks := map[int]bool{}
	auxiliaryBlocks := map[int]bool{}
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
	if w := m.historyWindow(s.Id); w.floor > 0 && m.historyStart[s.Id] <= w.floor {
		sequence = w.floor
		add(muted.Render("Earlier display history was removed by the size limit. Agent context is preserved."), 0)
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
	liveOutput := m.activeWork()
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
		// Streamed output is only ever shown beneath a still-running tool of the
		// current run, yet a long session keeps every chunk it ever received. Fold
		// only what can be displayed: the rest costs a concatenation per chunk
		// whose result is discarded, which is invisible until the history is large.
		if e.Kind == "tool_output" && e.RequestId != "" && liveOutput && e.RunId == s.RunId {
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
			if text := m.cachedSummary(e, usage, started, max(1, m.view.Width)); text != "" {
				if replyIndex >= 0 {
					lines[replyIndex] += "\n\n" + text
				} else {
					add(text, e.TimeMs)
				}
			}
			if text, loading := m.inlineSummary(e); text != "" {
				auxiliaryBlocks[len(lines)] = loading
				add(text, e.TimeMs)
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
					if !background && activity.Kind == "command" && result == nil && e.RunId == s.RunId && liveOutput && toolOutput[key] != "" {
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
				if e.Kind == "tool_call" || e.Kind == "tool_result" || e.Kind == "approval" {
					inspectBlocks[len(lines)-1] = true
				}
				if e.RunId == s.RunId && (e.Kind == "tool_call" || e.Kind == "tool_result") {
					toolBlocks[len(lines)-1] = true
				}
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
	if start, loaded := m.historyStart[s.Id]; loaded && m.historyOpening[s.Id] && (conversationReady || start <= m.historyWindow(s.Id).floor) {
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
			if auxiliaryBlocks[i] {
				if m.auxiliaryLoadingRows == nil {
					m.auxiliaryLoadingRows = map[int]bool{}
				}
				m.auxiliaryLoadingRows[start+row] = true
			}
			if inspectBlocks[i] {
				m.toolRows[start+row] = sequences[i]
			}
			if toolBlocks[i] && strings.HasPrefix(ansi.Strip(rows[row]), "  •") {
				if m.workingToolRows == nil {
					m.workingToolRows = map[int]bool{}
				}
				m.workingToolRows[start+row] = true
			}
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
	} else {
		// SetContent pulls the offset back only when it passes the last line,
		// not the last line the view can scroll to. A transcript that grew
		// shorter under the reader, as live output folding back up does, would
		// otherwise stop partway down the screen.
		m.view.SetYOffset(m.view.YOffset)
	}
	if m.autoRestorePosition > 0 && len(m.historyPositions) > 0 && m.historyPositions[0] <= m.autoRestorePosition {
		for i, pos := range m.historyPositions {
			if pos >= m.autoRestorePosition {
				m.view.SetYOffset(i)
				m.autoRestorePosition = 0
				break
			}
		}
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
	if kind == "resume" {
		if m.resumePending[id] {
			return nil
		}
		if m.resumePending == nil {
			m.resumePending = map[string]bool{}
		}
		m.resumePending[id] = true
	}
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
		if kind == "resume" {
			r.resumeSession = id
		}
		if kind == "send" {
			r.inputSession, r.inputRequest, r.inputText = id, clientID, text
			r.status = receipt.GetStatus()
		}
		return r
	}
}
func (m *model) Update(msg tea.Msg) (next tea.Model, cmd tea.Cmd) {
	defer func() {
		// Focus can change in any of the view handlers, including asynchronous ones.
		// Keep the latest focus command even when a handler returns early.
		switch msg.(type) {
		case tea.KeyMsg, tea.MouseMsg, tea.FocusMsg:
			if m.composerBlink && m.input.Focused() {
				m.focusComposer()
			}
		}
		if m.blurred {
			m.input.Cursor.SetMode(cursor.CursorStatic)
			m.composerFocusCmd = nil
		}
		cmd = tea.Batch(cmd, m.composerFocusCmd)
		m.composerFocusCmd = nil
	}()
	if _, ok := msg.(cursor.BlinkMsg); ok {
		var blink tea.Cmd
		m.input.Cursor, blink = m.input.Cursor.Update(msg)
		if blink != nil {
			return m, blink
		}
	}

	// Terminal focus decides what the bright green is allowed to claim, so it is
	// read before any branch below can consume the message. Styles already
	// copied into live widgets are repainted here; widgets built later read the
	// step they are built under.
	switch msg.(type) {
	case bed.EditErrorMsg:
		m.showError(msg.(bed.EditErrorMsg).Err.Error())
		return m, nil
	case bed.CopyMsg:
		m.copyText(string(msg.(bed.CopyMsg)))
		return m, nil

	case tea.FocusMsg:
		m.terminalFocus(false)
	case tea.BlurMsg:
		m.terminalFocus(true)
	case tea.KeyMsg:
		// Typing restarts the blink phase. A cursor that happens to be blinked
		// off at the moment a character lands reads as lag.
		m.blinkFrom = m.pulse
	}
	switch v := msg.(type) {
	case libraryResult:
		return m, m.receiveLibrary(v)
	case memorySeed:
		if m.library != v.page || m.library.generation != v.generation {
			return m, nil
		}
		m.library.loading = false
		if v.err != nil {
			m.library.message = v.err.Error()
			return m, nil
		}
		if v.data.Memory == nil {
			m.library.message = "Memory snapshot unavailable"
			return m, nil
		}
		m.seedMemory = v.data.Memory.ID
		m.project = v.project
		m.library = nil
		m.openAccounts(true)
		m.accountConnection = v.project.Id
		return m, m.loadAccounts()
	}

	switch v := msg.(type) {
	case downloadDone:
		m.receiveDownload(v)
		return m, nil
	case downloadProgress:
		if m.download == v.job {
			m.download.received = v.received
			m.download.total = v.total
			m.download.started = v.started
			m.download.transferred = v.transferred
		}
		return m, nil
	}
	defer m.observeSessionNavigation()()
	if v, ok := msg.(cxzUpdateResult); ok {
		m.autoChecking = false
		m.autoState = v.state
		if v.err == nil {
			m.autoCandidate = v.path
		} else {
			m.autoState.Reason = v.err.Error()
		}
		if m.autoCandidate != "" {
			m.notice = "cxz update ready · restarts after 5 minutes without input or drafts"
		}
		return m, nil
	}

	defer m.debugUpdate(msg)()
	m.updateDeleteConfirmation(msg, time.Now())
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
	if v, ok := msg.(settingsUpstreamResult); ok {
		if m.settingsPage == v.page {
			v.page.upstream = v.versions
			v.page.upstreamLoading = false
		}
		return m, nil
	}
	if v, ok := msg.(auxiliaryAccounts); ok {
		if m.settingsPage != nil && m.settingsPage.auxiliary == v.page {
			v.page.busy = false
			if v.err != nil {
				v.page.message = v.err.Error()
			} else {
				v.page.accounts = v.accounts
				v.page.choice = 0
				v.page.message = ""
			}
		}
		return m, nil
	}
	if v, ok := msg.(auxiliaryResult); ok {
		return m, m.receiveAuxiliary(v)
	}
	if v, ok := msg.(mcpResult); ok {
		m.receiveMCP(v)
		return m, nil
	}
	if v, ok := msg.(settingsResult); ok {
		m.receiveSettings(v)
		return m, m.settingsUpstream()
	}
	if v, ok := msg.(logsResult); ok {
		if m.report == v.report {
			m.report.text = v.text
		}
		return m, nil
	}
	if v, ok := msg.(redactSent); ok {
		// A queued message is not in the conversation yet; the composer status
		// row says it is waiting, so the echo would be saying it twice.
		if v.err != nil || v.status == "queued" {
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
			m.showError(v.err.Error() + "; reenter the secret with /@redact")
		} else if v.status == "queued" {
			m.notice = "send · waiting for the agent (secret files swept after 8 hours idle)"
		} else {
			m.notice = "send · accepted (secret files swept after 8 hours idle)"
		}
		return m, nil
	}
	var activity tea.Cmd
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
		cxzupdate.FrontendReady(m.ctx)
		// Report the first input immediately after a pause; steady activity is
		// covered by the heartbeat. Capture the old session before navigation.
		if time.Since(m.lastUIInput) >= time.Second {
			m.lastActivityReport = time.Time{}
			m.lastUIInput = time.Now()
			activity = m.reportActivity()
		}
	}
	next, cmd = m.update(msg)
	if k, ok := msg.(tea.KeyMsg); ok && k.Paste {
		if token := m.mentionContext(); token != nil {
			m.mentionDismissed = token.signature
		}
		if token := m.inlineContext(); token != nil {
			m.inlineDismissed = token.signature
		}
	}
	m.pruneRedactions()
	m.pruneFileUploads()
	if _, ok := msg.(listing); ok {
		cmd = tea.Batch(cmd, m.watchResources())
	}
	if m.resourceRefreshAgain && !m.resourceRefreshRunning {
		m.resourceRefreshAgain = false
		cmd = tea.Batch(cmd, m.refresh())
	}
	return next, tea.Batch(activity, cmd, m.syncHostFiles(), m.syncPathHints())
}

func (m *model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if mouse, ok := msg.(tea.MouseMsg); ok && m.mentionMouse(mouse) {
		return m, nil
	}
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg, tea.WindowSizeMsg:
		m.bottomButtonHover = false
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		if handled, cmd := m.sessionNavigationKey(k); handled {
			return m, cmd
		}
	}
	if m.sessionArchive != nil {
		switch v := msg.(type) {
		case tea.KeyMsg:
			if v.Type == tea.KeyCtrlD && !v.Paste {
				return m, tea.Quit
			}
			return m, m.sessionArchiveKey(v)
		case tea.MouseMsg:
			return m, nil
		}
	}
	if k, ok := msg.(tea.KeyMsg); ok && !k.Paste && !m.terminalFocused() {
		switch k.String() {
		case "ctrl+d":
			return m, tea.Quit
		case "ctrl+c":
			m.copyFocusedText()
			return m, nil
		case "esc":
			if m.composerAvailable() && !m.panelFocus && !m.focusList && m.selectedComposerText() != "" {
				m.input.ClearSelection()
				return m, nil
			}
			if m.previewInteraction() && m.selectionValid() && m.textSelection.text() != "" {
				m.textSelection = nil
				return m, nil
			}
		}
	}

	switch v := msg.(type) {
	case tea.KeyMsg, tea.WindowSizeMsg, tea.BlurMsg:
		m.toolHover, m.toolClick = nil, nil
		if d := m.questionDialog; d != nil {
			d.hovering = false
		}
		m.panelHoverY = 0
		m.panelHintHover = ""
		m.codeHover = nil
		if m.filePreview != nil {
			m.filePreview.hover = ""
		}
		if m.errorDialog != nil {
			m.errorDialog.hover = ""
		}
	case tea.MouseMsg:
		m.toolHover = nil
		if v.Button != tea.MouseButtonWheelUp && v.Button != tea.MouseButtonWheelDown && v.X >= m.contentOffset() && v.X < m.contentOffset()+m.width && !m.projectView && !m.accountView {
			if seq := m.toolAtRow(v.Y); seq > 0 {
				m.toolHover = &toolSelector{m.current().Id, seq}
			}
		}
		if last := m.toolClick; last != nil && v.Action == tea.MouseActionPress && v.Button == tea.MouseButtonLeft && (v.X-m.contentOffset() != last.x || v.Y != last.y) {
			m.toolClick = nil
		}
		if v.Button == tea.MouseButtonWheelUp || v.Button == tea.MouseButtonWheelDown || v.Action == tea.MouseActionMotion && v.Button == tea.MouseButtonLeft {
			m.toolClick = nil
		}
		if m.questionHeight() > 0 && m.pasteDialog == nil {
			m.questionDialog.hovering = false
			l := m.questionLayout()
			if v.X >= l.x && v.X < l.x+l.width && v.Y >= l.y && v.Y < l.y+l.height {
				return m, m.questionMouse(v)
			}
			if v.Action == tea.MouseActionPress {
				m.questionDialog.suspended = true
			}
		}
		if handled, cmd := m.errorMouse(v); handled {
			return m, cmd
		}
		m.codeHover = nil
		if m.filePreview != nil {
			m.filePreview.hover = ""
		}
		if sel := m.textSelection; sel != nil && sel.dragging && (v.Action == tea.MouseActionMotion || v.Action == tea.MouseActionRelease) && (v.X-m.contentOffset() != sel.startX || v.Y != sel.startY) {
			m.toolClick = nil
		}
		if m.composerMouse(v) {
			return m, nil
		}
		if m.selectionMouse(v) {
			return m, nil
		}
		if handled, cmd := m.panelMouse(v); handled {
			return m, cmd
		}
		m.focusConversationMouse(v)
	}
	if m.library != nil {
		switch v := msg.(type) {
		case tea.KeyMsg:
			return m, m.libraryKey(v)
		case tea.MouseMsg:
			return m, m.libraryMouse(v)
		}
	}
	if m.memoryPage != nil {
		switch v := msg.(type) {
		case tea.KeyMsg:
			return m, m.memoryKey(v)
		case tea.MouseMsg:
			return m, m.memoryMouse(v)
		}
	}
	if m.workflow != nil && m.workflow.auxiliary != nil {
		if k, ok := msg.(tea.KeyMsg); ok {
			return m, m.workflowKey(k)
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
	if k, ok := msg.(tea.KeyMsg); ok {
		if m.errorFocused() {
			return m, m.errorKey(k)
		}
		if !k.Paste && k.String() == "tab" && m.errorVisible() && m.accountView && !m.accountAdding && !m.loginChoosing {
			m.focusError()
			return m, nil
		}
	}
	if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeyF19 && !k.Paste && m.workflow == nil && m.redactDialog == nil && m.pasteDialog == nil && m.restartConfirm == nil {
		return m, m.openSettings()
	}
	if k, ok := msg.(tea.KeyMsg); ok && !k.Paste && m.questionDialog != nil && m.pasteDialog == nil && m.redactDialog == nil && k.Type == tea.KeyF6 {
		d := m.questionDialog
		d.suspended = !d.suspended
		m.panelFocus = false
		if d.suspended {
			return m, m.focusComposer()
		}
		m.focusQuestion()
		return m, nil
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
	case archiveLoaded:
		if m.sessionArchive == v.page {
			v.page.loading = false
			v.page.items = v.items
			if v.err != nil {
				v.page.message = v.err.Error()
			}
		}
		return m, nil
	case archiveRestored:
		if m.sessionArchive != v.page {
			return m, m.refresh()
		}
		v.page.busy = false
		if v.err != nil {
			v.page.message = v.err.Error()
			return m, nil
		}
		m.sessionArchive = nil
		m.accountView, m.accountChoosing = false, false
		return m.update(result{sessionID: v.session.Id, text: "Session restored; resume to continue"})
	case terminalChanged:
		m.closeSuccessfulTerminal(v.id)
		return m, nil
	case hostFileDue:
		return m, m.checkHostFiles(v)
	case hostFileChecked:
		return m, m.receiveHostFiles(v)
	case pathHintDue:
		return m, m.fetchPathHints(v)
	case pathHintResult:
		m.receivePathHints(v)
		return m, nil
	case tea.KeyMsg:
		if m.redactDialog != nil {
			return m, m.redactKey(v)
		}
		if v.Type == tea.KeyF20 && !v.Paste && !m.questionFocused() {
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
	case fileUploadProgress:
		if v.item.attachment == v.upload && m.pastes[v.item.token] == v.item {
			v.upload.received = v.received
			m.updateFileChip(v.item)
		}
		return m, nil
	case fileUploadDone:
		m.receiveFileUpload(v)
		return m, nil
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
				m.showError("Upload failed; original text retained: " + v.err.Error())
				return m, nil
			}
			if v.path == "" {
				m.showError("Upload returned no path; original text retained.")
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
		if p := v.flow.auxiliary; p != nil {
			m.busy = false
			if m.settingsPage == nil || m.settingsPage.auxiliary != p {
				return m, nil
			}
			if v.err != nil {
				p.message = v.err.Error()
				return m, nil
			}
			p.step, p.choice, p.models = "model", 0, nil
			return m, m.auxiliaryRequest(auxiliary.Request{Action: "models", Profile: p.draft}, p)
		}
		if v.flow.create {
			if v.err != nil {
				m.seedMemory = v.flow.seed
			}
			r := result{text: "session created", err: v.err}
			if v.session != nil && v.err == nil {
				r.sessionID = v.session.Id
			}
			return m.Update(r)
		}
		return m.Update(accountLoggedIn{v.err})
	case contextSent:
		if c := m.contextCapture; c != nil && c.request == v.request && v.err != nil {
			c.report.contextNote("Context unavailable: " + v.err.Error())
			m.contextCapture = nil
		}
		return m, nil
	case modelCatalogLoaded:
		return m, m.acceptModelCatalog(v)
	case historyWindowPage:
		return m, m.applyWindowPage(v)
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
			return m, tea.Batch(m.requestOlderHistory(v.initial || m.autoRestorePosition > 0), background)
		}
		return m, background
	case backgroundHistory:
		if !m.validSessionWatch(v.id, v.epoch) {
			return m, nil
		}
		if m.backgroundErrors == nil {
			m.backgroundErrors = map[string]string{}
		}
		if v.err == nil {
			m.storeBackgroundSnapshot(v.id, v.snapshot)
		}
		delete(m.backgroundLoading, v.id)
		delete(m.backgroundErrors, v.id)
		if v.err != nil {
			m.backgroundErrors[v.id] = v.err.Error()
		}
		m.render()
		return m, nil
	case sessionBackgroundChecked:
		m.receiveSessionBackground(v)
		return m, m.refreshSessionBackground()
	case completionChecked:
		return m, m.receiveCompletion(v)
	case soundRequested:
		return m, m.playNotification(v.sound)
	case sessionDeleted:
		m.receiveSessionDeleted(v)
		return m, m.refresh()
	case pulseTick:
		m.pulse++
		return m, pulseTimer()
	case permissionResult:
		delete(m.permissionUpdating, v.id)
		if current := m.current(); current != nil && current.Id == v.id && current.RunId == v.run {
			if v.err != nil {
				m.notice = "Permission update failed; refresh to check the saved policy: " + v.err.Error()
				if status.Code(v.err) == codes.Unimplemented {
					m.notice = "Permission RPC unavailable: update cxz on the host, run cxz install --recreate, then cxz project recreate WORKSPACE (replaces container; writable layer lost). See docs/updates.md.\nRPC error: " + v.err.Error()
				}
				m.showError(m.notice)
			} else {
				m.notice = "Permission " + v.mode + " saved for this session"
			}
		}
		return m, m.refresh()
	case approvalResult:
		key := v.id + "/" + v.run + "/" + v.request
		d := m.questionDrafts[key]
		if current := m.questionDialog; current != nil && current.id == v.id && current.run == v.run && current.request == v.request {
			d = current
		}
		if d != nil {
			if v.err == nil {
				delete(m.questionDrafts, key)
				if m.questionDialog == d {
					m.closeQuestion()
				}
			} else {
				d.sending = false
				d.message = "Answer failed: " + v.err.Error() + ". Not retried automatically."
				delete(m.approvalSent, key)
			}
		}
		if v.err != nil {
			m.showError("Approval failed; not retried. " + v.err.Error())
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
			m.focusComposer()
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
		if m.restartConfirm != nil {
			return m, m.restartMouse(v)
		}
		if m.report != nil {
			if m.contextReportMouse(v) {
				return m, nil
			}
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
		if handled, cmd := m.elicitationMouse(v); handled {
			return m, cmd
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
			if handled, cmd := m.bottomButtonMouse(v); handled {
				return m, cmd
			}
			if m.pinnedPromptMouse(v) {
				return m, nil
			}
			if m.codeBlockMouse(v) {
				return m, nil
			}
			if m.toolMouse(v, time.Now()) {
				return m, nil
			}
			m.beginSelection(v)
			m.view, _ = m.view.Update(v)
			if v.Button == tea.MouseButtonWheelUp || v.Button == tea.MouseButtonWheelDown {
				if s := m.current(); s != nil {
					m.historyWindow(s.Id).direction = map[bool]int{true: -1, false: 1}[v.Button == tea.MouseButtonWheelUp]
				}
				return m, m.loadOlderHistory()
			}
			return m, nil
		}
		return m, nil
	case renameResult:
		m.renameBusy = false
		if v.err != nil {
			m.showError(v.err.Error())
			return m, nil
		}
		if v.project {
			title := v.alias + m.connectionLabel(v.id)
			for _, p := range append(append([]*api.Project(nil), m.panelProjects...), m.project) {
				if p != nil && p.Id == v.id {
					p.Name = title
				}
			}
			for _, session := range append(append([]*api.Session(nil), m.sessions...), m.allSessions...) {
				if session.ProjectId == v.id {
					session.ProjectName = title
				}
			}
			m.renaming = false
			m.aliasInput.Blur()
			m.notice = "project title updated"
			return m, m.refresh()
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
			m.showError(v.err.Error())
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
			m.showError(v.err.Error())
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
			m.showError(v.err.Error())
		}
		return m, nil
	case tea.WindowSizeMsg:
		cxzupdate.FrontendReady(m.ctx)
		if d := m.questionDialog; d != nil {
			d.reveal = true
		}
		m.terminalWidth = v.Width
		m.width = min(v.Width, maxViewWidth)
		m.height = v.Height

		m.resize()
		m.render()
	case tick:
		if cmd := m.pinnedRestart(); cmd != nil {
			return m, cmd
		}
		m.watch()
		var history tea.Cmd
		if m.historyOpening[m.watchID] {
			// Resume an unfinished opening when returning to a session whose
			// previous page arrived while another conversation was selected.
			history = m.loadOlderHistory()
		}
		return m, tea.Batch(timer(), m.periodicRefresh(), m.reportActivity(), m.frontendUpdate(), m.pollSettings(), m.pollLibrary(), m.pollAuxiliary(), history)
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
		completion := m.observeSessions(v.sessions)
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
				m.focusComposer()
				m.notice = "Selected approval resolved; review the next request before deciding"
			}
		}
		if m.selectedApproval() == nil {
			m.approvalOffset = 0
			if m.focusApproval {
				m.focusComposer()
			}
			m.focusApproval = false
		}
		m.resize()
		m.render()
		m.syncQuestion()
		return m, tea.Batch(completion, m.refreshSessionBackground())
	case received:
		m.receiveEvent(v, true)
	case caughtUp:
		if !m.validSessionWatch(v.id, v.epoch) {
			return m, nil
		}
		if v.prepared != nil {
			m.mergePreparedHistory(*v.prepared)
		}
		changed := false
		for _, e := range v.events {
			changed = changed || (e.Seq > m.cursor[v.id] && e.Kind != "raw")
			m.receiveEvent(received{v.id, e}, false)
		}
		if current := m.current(); changed && current != nil && current.Id == v.id {
			m.render()
		}
		return m, nil
	case sessionWatchEnded:
		if !m.validSessionWatch(v.id, v.epoch) {
			return m, nil
		}
		if w := m.sessionWatches[v.id]; w != nil {
			w.cancel()
			w.active = false
			w.retryAfter = time.Now().Add(3 * time.Second)
		}
		return m.update(disconnected{v.id, v.err})
	case disconnected:
		if c := m.contextCapture; c != nil && c.id == v.id {
			c.report.contextNote("Disconnected while querying context. Reopen /context after reconnecting.")
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
			m.showError(v.err.Error())
		} else {
			m.notice = "Agent restarted · same session · saved permission policy"
		}
		return m, m.refresh()
	case result:
		delete(m.resumePending, v.resumeSession)
		// A queued message has not entered the conversation yet: the composer
		// status row is where it waits, so the transcript's echo of it goes.
		if v.inputRequest != "" && (v.err != nil || v.status == "queued") {
			m.removePendingInput(v.inputSession, v.inputRequest)
			m.render()
		}
		// A refused message is the user's text, not ours to drop. It goes back
		// where it was typed, unless something has been typed since.
		if (v.err != nil || v.status == "unqueued") && v.inputText != "" && strings.TrimSpace(m.input.Value()) == "" && !m.creating {
			m.input.SetValue(v.inputText)
			m.resize()
		}
		m.busy = false
		if v.err != nil {
			m.showError(v.err.Error())
		} else {
			m.notice = v.text
			if v.sessionID != "" {
				m.wantID = v.sessionID
				m.panelFocus = false
				m.projectView = false
				m.accountView = false
				return m, tea.Batch(m.focusComposer(), m.refresh())
			}
		}
		return m, m.refresh()
	case tea.KeyMsg:
		m.lastUIInput = time.Now()
		if m.renaming {
			return m, m.renameKey(v)
		}
		if m.panelFocus && m.report == nil && !m.accountView && m.workflow == nil && !m.creating {
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
		if v.Type == tea.KeyCtrlP && !v.Paste && m.workflow == nil && m.restartConfirm == nil {
			return m, m.openPastes()
		}
		if handled, cmd := m.composerKey(v); handled {
			return m, cmd
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
		if m.questionFocused() {
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
			if m.mentionKey(v) {
				return m, nil
			}
			if handled, cmd := m.commandKey(v); handled {
				return m, cmd
			}
			if m.inlineKey(v) {
				return m, nil
			}
		}
		if handled, cmd := m.composerIndentKey(v); handled {
			return m, cmd
		}
		if v.String() == "ctrl+q" {
			m.focusPanel()
			return m, m.refresh()
		}
		if v.String() == "f2" && !m.creating {
			return m, m.startProjectRename()
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
			if s := m.current(); s != nil {
				m.historyWindow(s.Id).direction = map[bool]int{true: -1, false: 1}[v.String() == "pgup"]
			}
			m.view, _ = m.view.Update(v)
			return m, m.loadOlderHistory()
		case "ctrl+end":
			return m, m.goToLatest()
		case "ctrl+home":
			if s := m.current(); s != nil {
				m.historyWindow(s.Id).direction = -1
			}
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
		case "f4":
			return m, m.action("interrupt", "")
		case "ctrl+r":
			return m, m.action("resume", "")
		case "ctrl+x":
			// The same key clears what you are writing. With nothing to clear it
			// takes back the message that is waiting, which is the other draft
			// you have -- the one cxz is holding for the agent.
			if s := m.current(); m.input.Value() == "" && s != nil && s.Queued != "" {
				return m, m.cancelQueued()
			}
			m.input.Reset()
			return m, nil
		case "right":
			if !m.focusList && !m.creating && m.input.Value() == "" {
				if text := m.suggestion(); text != "" {
					m.input.SetValue(safeText(text))
					m.input.CursorEnd()
					m.resize()
					return m, nil
				}
			}
		case "alt+g":
			if !m.focusList && !m.creating {
				return m, m.applySuggestion()
			}
			return m, nil
		case "alt+enter", "ctrl+j":
			if !m.focusList && !m.creating {
				if err := m.input.InsertNewline(); err != nil {
					m.showError(err.Error())
				}
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
				if err := m.input.InsertNewline(); err != nil {
					m.showError(err.Error())
				}
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
				return m, m.focusComposer()
			}
			text := strings.TrimSpace(m.input.Value())
			if !m.fileAttachmentsReady(text) {
				return m, nil
			}
			if text == "/redact" {
				m.input.Reset()
				m.notice = "Use /@redact inside your message, then Enter"
				return m, nil
			}
			if strings.HasPrefix(text, "/redact ") || strings.HasPrefix(text, "/redact\n") || strings.HasPrefix(text, "/redact\t") {
				m.input.Reset()
				m.notice = "Use /@redact inside your message; enter the secret only in its dialog"
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
			if localName == "/download" {
				return m, m.downloadCommand(text)
			}
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
			if localName == "/title" {
				return m, m.titleCommand(text)
			}
			if localName == "/summary" || localName == "/suggest" {
				return m, m.auxiliaryCommand(text)
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
		m.input, cmd = m.input.UpdateText(msg)
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
	// The step the keyboard marks belongs to whoever is drawing, so it is set
	// from this model rather than left wherever the last focus change put it.
	// terminalFocus still runs on the change itself, to repaint the widgets that
	// hold a copy of the style instead of reading it.
	keyboardStep(m.blurred)
	if m.sessionArchive != nil {
		return m.sessionArchiveScreen()
	}
	if m.library != nil {
		return m.libraryScreen()
	}
	if m.memoryPage != nil {
		return m.memoryScreen()
	}
	if m.workflow != nil && m.workflow.auxiliary != nil {
		return m.workflowScreen()
	}
	if m.settingsPage != nil {
		return m.settingsScreen()
	}
	defer func() { out = m.wideScreen(out) }()
	defer func() { out = m.errorOverlay(out) }()
	m.anchorCursor()
	if m.workflow != nil {
		return m.workflowScreen()
	}
	if m.width > 0 && (m.width < 40 || m.height < 14) {
		return screen("cxz\nResize terminal to 40 × 14 or larger.\nCtrl+D detach", m.width, m.height)
	}
	if (m.panelFocus || m.projectView) && !m.accountView && !m.creating && !m.panelVisible() {
		return m.reportView(m.panelScreen())
	}
	if m.accountView {
		return m.accountScreen()
	}
	if m.projectView && !m.creating {
		body := indentBlock("Select a session from Projects.\n\nn new session · a accounts")
		if notice := m.navigationNotice(); notice != "" {
			body += "\n\n" + indentBlock(warning.Render(ansi.Hardwrap(safeText(notice), max(1, m.width-4), true)))
		}
		if m.report != nil {
			body += strings.Repeat("\n", m.height)
		}
		return m.reportView(screen(body, m.width, m.height))
	}
	return m.sessionScreen()
}

func (m *model) receiveEvent(v received, repaint bool) {
	anchor, follow := m.historyAnchor(), m.view.AtBottom()
	if floor := core.HistoryFloor(v.event.Kind, v.event.Payload); floor > 0 {
		m.applyHistoryFloor(v.id, floor)
	}
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
		w := m.historyWindow(v.id)
		if !w.detached {
			w.tail = v.event.Seq
		}
		trimmed := false
		if !w.detached {
			m.events[v.id] = append(m.events[v.id], v.event)
			trimmed = m.limitHistory(v.id, false, 0)
		}
		// A catalog published while the picker is open replaces what it shows --
		// including the one a refresh asked for, which arrives this way rather
		// than as a reply, so nothing had to wait on the provider.
		if v.event.Kind == "models" {
			var catalog modelCatalog
			if json.Unmarshal(v.event.Payload, &catalog) == nil {
				catalog.seq, catalog.ms = v.event.Seq, v.event.TimeMs
				m.rememberCatalog(v.id, v.event.RunId, &catalog)
				if p := m.modelPicker; p != nil && !p.loading && p.id == v.id && p.run == v.event.RunId {
					selected := ""
					options := p.options()
					if p.selected < len(options) {
						selected = options[p.selected]
					}
					p.catalog, p.refreshing, p.message = &catalog, false, ""
					p.selected = 0
					for i, option := range p.options() {
						if option == selected {
							p.selected = i
							break
						}
					}
				}
			}
		}
		if s := m.current(); s != nil && s.Id == v.id && repaint && v.event.Kind != "raw" {
			m.render()
			if !follow && trimmed {
				m.restoreHistoryAnchor(anchor)
			}
		}
	}
}
