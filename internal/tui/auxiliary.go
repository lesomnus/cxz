package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"github.com/lesomnus/cxz/resource"
	"slices"
	"strings"
	"time"
)

type auxiliaryPage struct {
	loginIfNeeded bool
	config        auxiliary.Config
	selected      int
	busy          bool
	editing       bool
	step          string
	choice        int
	draft         auxiliary.Profile
	models        []agentview.ModelOption
	accounts      []*resource.Account
	message       string
}
type auxiliaryResult struct {
	version                     uint64
	page                        *auxiliaryPage
	connection, session, action string
	reply                       auxiliary.Reply
	err                         error
}

func (m *model) openAuxiliary() tea.Cmd {
	m.settingsPage.auxiliary = &auxiliaryPage{}
	return m.auxiliaryRequest(auxiliary.Request{Action: "list"}, m.settingsPage.auxiliary)
}
func (m *model) auxiliaryRequest(r auxiliary.Request, p *auxiliaryPage) tea.Cmd {
	connection := m.connectionRef()
	if p != nil {
		connection = m.settingsPage.connection
		p.busy = true
	}
	action := r.Action
	if action == "apply" {
		r.Action = "status"
	}
	version := m.auxiliaryVersions[connection+"/"+r.Session]
	ctx, client := m.contextFor(connection), m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		v := auxiliaryResult{version: version, page: p, connection: connection, session: r.Session, action: action}
		v.reply, v.err = auxCall(ctx, client, r)
		return v
	}
}

// auxCall is the one place the view's request becomes aux calls. The view keeps
// asking in its own terms -- a row, a task, a session -- and each of those is
// now a method rather than an action string inside a payload.
func auxCall(ctx context.Context, client api.SessionsClient, r auxiliary.Request) (auxiliary.Reply, error) {
	switch r.Action {
	case "list":
		out, err := client.AuxConfig(ctx, &api.Empty{})
		if err != nil {
			return auxiliary.Reply{}, err
		}
		return auxiliary.Reply{Config: auxConfigOf(out)}, nil
	case "put":
		out, err := client.AuxSetConfig(ctx, &api.AuxSetConfigInput{Profiles: []*api.AuxProfile{{
			Kind: r.Task, Enabled: r.Profile.Enabled, Account: r.Profile.Account,
			Model: r.Profile.Model, Effort: r.Profile.Effort,
		}}})
		if err != nil {
			return auxiliary.Reply{}, err
		}
		return auxiliary.Reply{Config: auxConfigOf(out), Message: out.Message}, nil
	case "models":
		out, err := client.AuxModels(ctx, &api.AuxModelsInput{Account: r.Profile.Account})
		if err != nil {
			return auxiliary.Reply{}, err
		}
		reply := auxiliary.Reply{NeedsLogin: out.NeedsLogin}
		for _, o := range out.Models {
			reply.Models = append(reply.Models, agentview.ModelOption{
				ID: o.Id, ResolvedID: o.ResolvedId, Name: o.Name,
				Efforts: o.Efforts, DefaultEffort: o.DefaultEffort, Default: o.Default,
			})
		}
		return reply, nil
	case "status":
		out, err := client.AuxStatus(ctx, &api.AuxStatusInput{SessionId: r.Session})
		return auxStateOf(out), err
	case "title":
		out, err := client.AuxRun(ctx, &api.AuxRunInput{SessionId: r.Session, Kinds: []string{"title"}, Text: r.Text})
		return auxStateOf(out), err
	case "session":
		if r.Enabled != nil {
			out, err := client.AuxPrefer(ctx, &api.AuxPreferInput{
				SessionId:   r.Session,
				Preferences: []*api.AuxPreference{{Kind: r.Task, Enabled: *r.Enabled}},
			})
			return auxStateOf(out), err
		}
		out, err := client.AuxRun(ctx, &api.AuxRunInput{SessionId: r.Session, Kinds: []string{r.Task}})
		return auxStateOf(out), err
	}
	return auxiliary.Reply{}, fmt.Errorf("unknown auxiliary request %q", r.Action)
}

func auxConfigOf(out *api.AuxConfigReply) auxiliary.Config {
	cfg := auxiliary.Config{Revision: out.Revision}
	for _, p := range out.Profiles {
		profile := auxiliary.Profile{
			Enabled: p.Enabled, Account: p.Account, Agent: p.Agent,
			Backend: p.Backend, Model: p.Model, Effort: p.Effort,
		}
		switch p.Kind {
		case "summary":
			cfg.Summary, cfg.Since = profile, p.SinceMs
		case "suggestion":
			cfg.Suggestion, cfg.Since = profile, p.SinceMs
		case "title":
			cfg.Title, cfg.TitleSince = profile, p.SinceMs
		}
	}
	return cfg
}

func auxStateOf(out *api.AuxState) auxiliary.Reply {
	if out == nil {
		return auxiliary.Reply{}
	}
	reply := auxiliary.Reply{Message: out.Message, Job: auxJobOf(out.Current)}
	if out.Title != "" {
		reply.Title = &auxiliary.TitleState{Text: out.Title}
	}
	for _, s := range out.Summaries {
		reply.Summaries = append(reply.Summaries, auxiliary.Summary{Run: s.RunId, Turn: s.Turn, Text: s.Text})
	}
	// Only an answer that reported preferences replaces the ones held: a reply
	// that said nothing about them is not a reply that said they are off.
	if len(out.Preferences) > 0 {
		cfg := auxiliary.SessionConfig{}
		for _, p := range out.Preferences {
			switch p.Kind {
			case "summary":
				cfg.Summary, cfg.Since = p.Enabled, p.SinceMs
			case "suggestion":
				cfg.Suggestion, cfg.Since = p.Enabled, p.SinceMs
			}
		}
		reply.SessionConfig = &cfg
	}
	return reply
}

// auxJobOf reads the kinds a task was asked for and the results it has so far,
// which is how a run that already produced its summary shows it while the
// suggestion beside it is still being generated.
func auxJobOf(a *api.Aux) *auxiliary.Job {
	if a == nil {
		return nil
	}
	job := &auxiliary.Job{
		ID: a.Id, Session: a.SessionId, Run: a.RunId, Turn: a.Turn,
		Revision: a.Revision, Status: a.State, Error: a.Message,
	}
	for _, kind := range a.Kinds {
		switch kind {
		case "summary":
			job.SummaryRequested = true
		case "suggestion":
			job.SuggestionRequested = true
		}
	}
	for _, r := range a.Results {
		switch r.Kind {
		case "summary":
			job.Summary = r.Text
		case "suggestion":
			job.Suggestion = r.Text
		}
	}
	for _, u := range a.Usage {
		job.Usage = append(job.Usage, auxiliary.Usage{Account: u.Account, Model: u.Model, Task: u.Kind, Data: u.Data})
	}
	return job
}
func (m *model) receiveAuxiliary(v auxiliaryResult) tea.Cmd {
	if v.page != nil {
		if m.settingsPage == nil || m.settingsPage.auxiliary != v.page {
			return nil
		}
		p := v.page
		p.busy = false
		autoLogin := p.loginIfNeeded
		p.loginIfNeeded = false
		if v.err != nil {
			p.message = v.err.Error()
			return nil
		}
		// A catalog read does not carry the settings, so the page keeps the
		// ones it already has.
		if v.action != "models" {
			p.config = v.reply.Config
		}
		if v.action == "models" {
			if v.reply.NeedsLogin {
				p.models = nil
				if autoLogin {
					return m.loginAuxiliaryAccount(p)
				}
				p.message = "Account needs login. Press l to log in or Esc to cancel."
				return nil
			}
			p.models = v.reply.Models
			p.step, p.choice = "model", 0
			p.message = ""
			if len(p.models) == 0 {
				p.message = "No models available. Press r to retry or Esc to choose another account."
			}
		} else {
			p.editing = false
			p.message = "New completed turns use these settings; no historical backfill."
			if v.reply.Message != "" {
				p.message = v.reply.Message
			}
		}
		return nil
	}
	if v.action == "status" {
		m.auxiliaryPolling = false
	}
	key := v.connection + "/" + v.session
	if v.version != m.auxiliaryVersions[key] {
		return nil
	}
	if v.action == "session" {
		delete(m.auxiliaryPending, key)
	}
	if v.connection != m.connectionRef() {
		return nil
	}
	if m.auxiliaryJobs == nil {
		m.auxiliaryJobs = map[string]*auxiliary.Job{}
	}
	if v.err != nil {
		m.auxiliaryError = v.err.Error()
		if v.action == "session" || v.action == "title" {
			if j := m.auxiliaryJobs[key]; j != nil && j.ID == "pending" {
				j.Status = "failed"
				j.Error = v.err.Error()
			}
			m.notice = safeText(v.err.Error())
			m.render()
		}
		return nil
	}
	m.auxiliaryError = ""
	if v.action == "title" {
		m.notice = v.reply.Message
		return m.refresh()
	}
	old := m.auxiliaryJobs[key]
	changedSummaries := !slices.Equal(m.auxiliarySummaries[key], v.reply.Summaries)
	if m.auxiliarySummaries == nil {
		m.auxiliarySummaries = map[string][]auxiliary.Summary{}
	}
	m.auxiliarySummaries[key] = v.reply.Summaries
	if v.action == "session" && v.reply.Message != "" {
		m.notice = safeText(v.reply.Message)
	}
	if v.reply.SessionConfig != nil {
		if m.auxiliaryConfigs == nil {
			m.auxiliaryConfigs = map[string]auxiliary.SessionConfig{}
		}
		m.auxiliaryConfigs[key] = *v.reply.SessionConfig
	}
	m.auxiliaryJobs[v.connection+"/"+v.session] = v.reply.Job
	if v.action == "apply" {
		if current := m.current(); current != nil && current.Id == v.session && m.input.Value() == "" {
			if text := m.suggestion(); text != "" {
				m.input.SetValue(text)
				m.input.CursorEnd()
				m.notice = "AI suggestion copied; review before sending"
			} else {
				m.notice = "No current AI suggestion"
			}
		}
	}
	if current := m.current(); current != nil && current.Id == v.session && (changedSummaries || !sameAuxiliaryJob(old, v.reply.Job)) {
		m.render()
	}

	return nil
}
func (m *model) pollAuxiliary() tea.Cmd {
	if m.auxiliaryPolling || time.Since(m.auxiliaryChecked) < time.Second || m.ctx == nil {
		return nil
	}
	s := m.current()
	if s == nil {
		return nil
	}
	if m.auxiliaryPending[m.connectionRef()+"/"+s.Id] {
		return nil
	}
	m.auxiliaryPolling = true
	m.auxiliaryChecked = time.Now()
	return m.auxiliaryRequest(auxiliary.Request{Action: "status", Session: s.Id}, nil)
}
func (m *model) auxiliaryJob() *auxiliary.Job {
	s := m.current()
	if s == nil {
		return nil
	}
	return m.auxiliaryJobs[m.connectionRef()+"/"+s.Id]
}
func (m *model) suggestion() string {
	s := m.current()
	j := m.auxiliaryJob()
	if s == nil || j == nil || (j.Status != "completed" && !(auxiliaryLoading(j) && !j.SuggestionRequested)) || j.Run != s.RunId || s.State != "idle" || len(m.pendingInputs[s.Id]) > 0 {
		return ""
	}
	for _, e := range m.events[s.Id] {
		if e.Kind == "input" && e.Seq > j.Turn {
			return ""
		}
	}
	return j.Suggestion
}
func (m *model) applySuggestion() tea.Cmd {
	if m.input.Value() != "" {
		m.notice = "Clear the draft before applying an AI suggestion"
		return nil
	}
	s := m.current()
	if s == nil {
		return nil
	}
	return m.auxiliaryRequest(auxiliary.Request{Action: "apply", Session: s.Id}, nil)
}
func (m *model) auxiliaryKey(k tea.KeyMsg) tea.Cmd {
	p := m.settingsPage.auxiliary
	if k.Paste {
		return nil
	}
	if k.String() == "esc" {
		if p.busy {
			return nil
		}
		if p.editing {
			p.editing = false
			p.message = ""
		} else {
			m.settingsPage.auxiliary = nil
		}
		return nil
	}
	if p.busy {
		return nil
	}
	task := p.task()
	profile := map[string]auxiliary.Profile{"summary": p.config.Summary, "suggestion": p.config.Suggestion, "title": p.config.Title}[task]
	if p.editing {
		choices := p.choices()
		switch k.String() {
		case "up", "shift+tab":
			p.choice = max(0, p.choice-1)
		case "down", "tab":
			p.choice = min(max(0, len(choices)-1), p.choice+1)
		case "l":
			if p.step == "model" {
				return m.loginAuxiliaryAccount(p)
			}
		case "r":
			if p.step == "account" {
				return m.loadAuxiliaryAccounts(p)
			}
			if p.step == "model" {
				return m.auxiliaryRequest(auxiliary.Request{Action: "models", Profile: p.draft}, p)
			}
		case "enter":
			if p.choice >= len(choices) {
				return nil
			}
			switch p.step {
			case "account":
				// A profile another task already uses needs no catalog and no
				// login probe: that task validated it, and the point of copying
				// it whole is that the two end up equal field for field.
				if reuse := p.reusable(task); p.choice < len(reuse) {
					p.draft = reuse[p.choice].profile
					return m.auxiliaryRequest(auxiliary.Request{Action: "put", Task: task, Profile: p.draft}, p)
				}
				a := p.accounts[p.choice-len(p.reusable(task))]
				p.draft = auxiliary.Profile{Enabled: true, Account: a.GetAlias(), Agent: a.GetAgent(), Backend: a.GetAuthBackend()}
				p.step, p.choice, p.models = "model", 0, nil
				p.loginIfNeeded = true
				p.message = ""
				return m.auxiliaryRequest(auxiliary.Request{Action: "models", Profile: p.draft}, p)
			case "model":
				p.draft.Model, p.draft.Effort = p.models[p.choice].ID, ""
				p.step, p.choice = "effort", 0
			case "effort":
				p.draft.Effort = ""
				if p.choice > 0 {
					p.draft.Effort = choices[p.choice]
				}
				return m.auxiliaryRequest(auxiliary.Request{Action: "put", Task: task, Profile: p.draft}, p)
			}
		}
		return nil
	}
	switch k.String() {
	case "up", "down", "tab":
		if k.String() == "up" {
			p.selected = (p.selected + 2) % 3
		} else {
			p.selected = (p.selected + 1) % 3
		}
	case "enter", "e":
		p.editing = true
		p.step = "account"
		p.choice = 0
		p.message = ""
		p.draft = auxiliary.Profile{}
		return m.loadAuxiliaryAccounts(p)
	case " ":
		if profile.Account == "" {
			return m.auxiliaryKey(tea.KeyMsg{Type: tea.KeyEnter})
		}
		profile.Enabled = !profile.Enabled
		return m.auxiliaryRequest(auxiliary.Request{Action: "put", Task: task, Profile: profile}, p)
	case "r":
		return m.auxiliaryRequest(auxiliary.Request{Action: "list"}, p)
	}
	return nil
}
func (m *model) loginAuxiliaryAccount(p *auxiliaryPage) tea.Cmd {
	p.loginIfNeeded = false
	m.accountConnection = m.settingsPage.connection
	return m.startAccountWorkflow(p.draft.Account, p.draft.Agent, "", false, p)
}

// task is which row the page is on, named rather than numbered so the wizard
// and the list cannot disagree about what is being edited.
func (p *auxiliaryPage) task() string {
	return []string{"summary", "suggestion", "title"}[min(max(0, p.selected), 2)]
}

// auxiliaryReuse is another task's profile, offered as one choice. Summary and
// suggestion are generated in a single call when their profiles are equal --
// every field, not only the model -- so picking the other task's profile from a
// list is both fewer keystrokes than walking the wizard and the only way to be
// sure the two really match.
type auxiliaryReuse struct {
	from    string
	profile auxiliary.Profile
}

func (r auxiliaryReuse) label() string {
	where := r.profile.Account + "/" + r.profile.Model
	if r.profile.Effort != "" {
		where += "/" + r.profile.Effort
	}
	return "use " + where + " from " + r.from
}

// reusable lists the profiles the other tasks already use, most useful first:
// for summary or suggestion, the other half of the pair that can share a call
// comes before the title's. A profile without a model was never configured, and
// a duplicate of one already listed would be the same choice twice.
func (p *auxiliaryPage) reusable(task string) []auxiliaryReuse {
	order := map[string][]string{
		"summary":    {"suggestion", "title"},
		"suggestion": {"summary", "title"},
		"title":      {"summary", "suggestion"},
	}[task]
	named := map[string]auxiliary.Profile{"summary": p.config.Summary, "suggestion": p.config.Suggestion, "title": p.config.Title}
	labels := map[string]string{"summary": "Summary", "suggestion": "Next-message suggestion", "title": "Session title"}
	var out []auxiliaryReuse
	for _, other := range order {
		profile := named[other]
		if profile.Account == "" || profile.Model == "" {
			continue
		}
		profile.Enabled = true
		if slices.ContainsFunc(out, func(v auxiliaryReuse) bool { return v.profile == profile }) {
			continue
		}
		out = append(out, auxiliaryReuse{from: labels[other], profile: profile})
	}
	return out
}

func (p *auxiliaryPage) choices() []string {
	var out []string
	switch p.step {
	case "account":
		for _, v := range p.reusable(p.task()) {
			out = append(out, v.label())
		}
		for _, a := range p.accounts {
			out = append(out, a.GetAlias()+" · "+a.GetAgent()+" · "+a.GetName())
		}
	case "model":
		for _, v := range p.models {
			out = append(out, v.ID+" · "+v.Name)
		}
	case "effort":
		out = append(out, "Provider default")
		for _, v := range p.models {
			if v.ID == p.draft.Model {
				out = append(out, v.Efforts...)
				break
			}
		}
	}
	return out
}

type auxiliaryAccounts struct {
	page     *auxiliaryPage
	accounts []*resource.Account
	err      error
}

func (m *model) loadAuxiliaryAccounts(p *auxiliaryPage) tea.Cmd {
	p.busy = true
	service := m.accountClient()
	if c, ok := m.client.(interface {
		AccountClient(string) resource.AccountServiceClient
	}); ok {
		service = c.AccountClient(m.settingsPage.connection)
	}
	ctx := m.contextFor(m.settingsPage.connection)
	return func() tea.Msg {
		if service == nil {
			return auxiliaryAccounts{page: p, err: fmt.Errorf("Account service unavailable")}
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		var all []*resource.Account
		after := ""
		for {
			page, err := service.List(ctx, resource.AccountListRequest_builder{Size: 200, After: after}.Build())
			if err != nil {
				return auxiliaryAccounts{page: p, err: err}
			}
			all = append(all, page.GetItems()...)
			if page.GetNext() == "" {
				break
			}
			after = page.GetNext()
		}
		return auxiliaryAccounts{page: p, accounts: all}
	}
}

func (m *model) auxiliaryScreen() string {
	p := m.settingsPage.auxiliary
	lines := []string{"", accent.Bold(true).Render("AI tasks" + m.connectionLabel(m.settingsPage.connection)), ""}
	if p.editing {
		lines = append(lines, "Select "+p.step+" · "+p.draft.Account)
		choices := p.choices()
		capacity := max(1, m.height-12)
		start := max(0, p.choice-capacity+1)
		for i := start; i < min(len(choices), start+capacity); i++ {
			line := "  " + safeText(choices[i])
			if i == p.choice {
				line = focus.Render("› " + safeText(choices[i]))
			}
			lines = append(lines, line)
		}
		if len(choices) == 0 && p.step == "account" {
			lines = append(lines, "No accounts. Add an account in Settings → Accounts, then press r.")
		}
		lines = append(lines, "↑/↓ select · Enter continue / save effort · r retry · l log in (model step) · Esc cancel")
	} else {
		for i, profile := range []auxiliary.Profile{p.config.Summary, p.config.Suggestion, p.config.Title} {
			name := []string{"Summary", "Next-message suggestion", "Session title"}[i]
			state := "Default off"
			if profile.Enabled {
				state = "Default on"
			}
			line := fmt.Sprintf("  %-24s %s · %s · %s · %s", name, state, profile.Account, profile.Model, profile.Effort)
			if i == p.selected {
				line = focus.Render("›" + line[1:])
			}
			lines = append(lines, line)
		}
		lines = append(lines, "", "Enter edit · Space change default · r refresh · Esc back")
	}
	// Whether the pair shares a call is not visible in the rows above -- the
	// profiles have to be equal in every field, and two rows that read alike can
	// still differ -- so the one thing it changes is stated.
	if p.config.Summary.Enabled && p.config.Summary == p.config.Suggestion {
		lines = append(lines, "", accent.Render("Summary and suggestion share a profile: one call per turn, one copy of the conversation."))
	} else if p.config.Summary.Enabled && p.config.Suggestion.Enabled {
		lines = append(lines, "", muted.Render("Summary and suggestion differ: a call each. Edit one and choose \"use … from …\" to share a call."))
	}
	lines = append(lines, "", "Checkpoint uses the summary account, or suggestion account when summary is off.", "/summary and /suggest: on/off per session, or once while off.")
	if p.busy {
		lines = append(lines, "Checking account/model…")
	}
	lines = append(lines, strings.Split(safeText(p.message), "\n")...)
	for i := range lines {
		lines[i] = "  " + clip(lines[i], max(1, m.settingsWidth()-4))
	}
	return screen(strings.Join(lines, "\n"), m.settingsWidth(), m.height)
}

func sameAuxiliaryJob(a, b *auxiliary.Job) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ID == b.ID && a.Status == b.Status && a.Summary == b.Summary && a.Suggestion == b.Suggestion && a.Error == b.Error && a.SummaryRequested == b.SummaryRequested && a.SuggestionRequested == b.SuggestionRequested
}
func auxiliaryDots(step int) string {
	n := step / 4 % 4
	return strings.Repeat(".", n) + strings.Repeat(" ", 3-n)
}
func auxiliaryLoading(j *auxiliary.Job) bool {
	return j != nil && (j.Status == "queued" || j.Status == "running")
}
func (m *model) inlineSummary(e *api.Event) (string, bool) {
	j := m.auxiliaryJob()
	text := ""
	if s := m.current(); s != nil {
		for _, summary := range m.auxiliarySummaries[m.connectionRef()+"/"+s.Id] {
			if summary.Run == e.RunId && summary.Turn == e.Seq {
				text = summary.Text
				break
			}
		}
	}
	if j == nil || j.Run != e.RunId || j.Turn != e.Seq {
		j = &auxiliary.Job{}
	}
	if j.Summary != "" {
		text = j.Summary
	}
	if j.Summary == "" && j.SummaryRequested {
		if auxiliaryLoading(j) {
			return indentBlock(muted.Render("Summary ...")), true
		}
		if j.Error != "" {
			text = j.Error
		}
	}
	if text == "" {
		return "", false
	}
	return indentBlock(muted.Render("Summary") + "\n" + markdownView(text, max(1, m.view.Width-2))), false
}
func (m *model) suggestionGhost() string {
	if m.input.Value() != "" {
		return ""
	}
	j, s := m.auxiliaryJob(), m.current()
	if j != nil && s != nil && j.Run == s.RunId && s.State == "idle" && len(m.pendingInputs[s.Id]) == 0 {
		for _, e := range m.events[s.Id] {
			if e.Kind == "input" && e.Seq > j.Turn {
				return ""
			}
		}
		if auxiliaryLoading(j) && j.SuggestionRequested {
			return "Suggestion " + auxiliaryDots(m.pulse+spinnerKeyPhase(j.ID))
		}
		if j.Status == "failed" && j.SuggestionRequested && j.Error != "" {
			return "Suggestion · " + safeText(j.Error)
		}
	}
	return m.suggestion()
}
func (m *model) auxiliaryCommand(text string) tea.Cmd {
	fields := strings.Fields(text)
	task := "summary"
	if fields[0] == "/suggest" {
		task = "suggestion"
	}
	if len(fields) > 2 || len(fields) == 2 && fields[1] != "on" && fields[1] != "off" {
		m.notice = "Usage: " + fields[0] + " [on|off]"
		return nil
	}
	s := m.current()
	if s == nil {
		m.notice = "Select a session first"
		return nil
	}
	key := m.connectionRef() + "/" + s.Id
	if m.auxiliaryPending[key] {
		return nil
	}
	r := auxiliary.Request{Action: "session", Session: s.Id, Task: task}
	if len(fields) == 2 {
		enabled := fields[1] == "on"
		r.Enabled = &enabled
	} else {
		cfg, known := m.auxiliaryConfigs[key]
		enabled := known && (task == "summary" && cfg.Summary || task == "suggestion" && cfg.Suggestion)
		if !enabled {
			if s.State != "idle" || len(m.pendingInputs[s.Id]) > 0 {
				m.notice = "Wait for the final response"
				return nil
			}
			if old := m.auxiliaryJob(); auxiliaryLoading(old) {
				if task == "summary" && old.SummaryRequested || task == "suggestion" && old.SuggestionRequested {
					return nil
				}
				queued := *old
				if task == "summary" {
					queued.SummaryRequested = true
				} else {
					queued.SuggestionRequested = true
				}
				m.auxiliaryJobs[key] = &queued
				m.render()
			} else {
				// Immediate presentation while the server validates/fetches the source.
				for i := len(m.events[s.Id]) - 1; i >= 0; i-- {
					e := m.events[s.Id][i]
					if e.Kind == "input" {
						break
					}
					if e.Kind == "turn_end" && e.Text == "completed" && e.RunId == s.RunId {
						j := &auxiliary.Job{ID: "pending", Run: e.RunId, Turn: e.Seq, Status: "queued", SummaryRequested: task == "summary", SuggestionRequested: task == "suggestion"}
						if old := m.auxiliaryJob(); old != nil && old.Turn == e.Seq && old.Run == e.RunId {
							if task != "summary" {
								j.Summary = old.Summary
							}
							if task != "suggestion" {
								j.Suggestion = old.Suggestion
							}
						}
						if m.auxiliaryJobs == nil {
							m.auxiliaryJobs = map[string]*auxiliary.Job{}
						}
						m.auxiliaryJobs[key] = j
						m.render()
						break
					}
				}
			}
		}
	}
	if m.auxiliaryVersions == nil {
		m.auxiliaryVersions = map[string]uint64{}
	}
	if m.auxiliaryPending == nil {
		m.auxiliaryPending = map[string]bool{}
	}
	m.auxiliaryVersions[key]++
	m.auxiliaryPending[key] = true
	return m.auxiliaryRequest(r, nil)
}

func (m *model) titleCommand(text string) tea.Cmd {
	s := m.current()
	if s == nil {
		m.notice = "Select a session first"
		return nil
	}
	r := auxiliary.Request{Action: "title", Session: s.Id}
	args := strings.TrimSpace(strings.TrimPrefix(text, "/title"))
	if args != "" {
		if !strings.HasPrefix(args, "set ") || strings.TrimSpace(strings.TrimPrefix(args, "set ")) == "" {
			m.notice = "Usage: /title or /title set <title>"
			return nil
		}
		r.Text = strings.TrimSpace(strings.TrimPrefix(args, "set "))
	}
	m.input.Reset()
	m.resize()
	return m.auxiliaryRequest(r, nil)
}
