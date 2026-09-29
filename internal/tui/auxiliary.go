package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"slices"
	"strings"
	"time"
)

type auxiliaryPage struct {
	config   auxiliary.Config
	selected int
	busy     bool
	editing  bool
	field    int
	inputs   []textinput.Model
	message  string
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
	return tea.Batch(m.auxiliaryRequest(auxiliary.Request{Action: "list"}, m.settingsPage.auxiliary), m.loadAccounts())
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
		b, _ := json.Marshal(r)
		out, e := client.Docker(ctx, &api.DockerInput{Action: "auxiliary", Spec: b})
		v := auxiliaryResult{version: version, page: p, connection: connection, session: r.Session, action: action, err: e}
		if e == nil {
			v.err = json.Unmarshal([]byte(out.Status), &v.reply)
		}
		return v
	}
}
func (m *model) receiveAuxiliary(v auxiliaryResult) {
	if v.page != nil {
		if m.settingsPage == nil || m.settingsPage.auxiliary != v.page {
			return
		}
		p := v.page
		p.busy = false
		if v.err != nil {
			p.message = v.err.Error()
			return
		}
		p.config = v.reply.Config
		if v.action == "models" {
			var lines []string
			for _, model := range v.reply.Models {
				lines = append(lines, model.ID+" · "+strings.Join(model.Efforts, ", "))
			}
			p.message = strings.Join(lines, "\n")
		} else {
			p.editing = false
			p.message = "New completed turns use these settings; no historical backfill."
			if v.reply.Message != "" {
				p.message = v.reply.Message
			}
		}
		return
	}
	if v.action == "status" {
		m.auxiliaryPolling = false
	}
	key := v.connection + "/" + v.session
	if v.version != m.auxiliaryVersions[key] {
		return
	}
	if v.action == "session" {
		delete(m.auxiliaryPending, key)
	}
	if v.connection != m.connectionRef() {
		return
	}
	if m.auxiliaryJobs == nil {
		m.auxiliaryJobs = map[string]*auxiliary.Job{}
	}
	if v.err != nil {
		m.auxiliaryError = v.err.Error()
		if v.action == "session" {
			if j := m.auxiliaryJobs[key]; j != nil && j.ID == "pending" {
				j.Status = "failed"
				j.Error = v.err.Error()
			}
			m.notice = safeText(v.err.Error())
			m.render()
		}
		return
	}
	m.auxiliaryError = ""
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
	if k.Paste && !p.editing {
		return nil
	}
	if k.String() == "esc" && !k.Paste {
		if p.editing {
			p.editing = false
		} else {
			m.settingsPage.auxiliary = nil
		}
		return nil
	}
	if p.busy {
		return nil
	}
	task := "summary"
	profile := p.config.Summary
	if p.selected == 1 {
		task = "suggestion"
		profile = p.config.Suggestion
	}
	if p.editing {
		if !k.Paste && (k.String() == "tab" || k.String() == "shift+tab") {
			p.inputs[p.field].Blur()
			d := 1
			if k.String() == "shift+tab" {
				d = -1
			}
			p.field = (p.field + d + 3) % 3
			return p.inputs[p.field].Focus()
		}
		if !k.Paste && k.String() == "ctrl+s" {
			return m.auxiliaryRequest(auxiliary.Request{Action: "put", Task: task, Profile: auxiliary.Profile{Enabled: true, Account: strings.TrimSpace(p.inputs[0].Value()), Model: strings.TrimSpace(p.inputs[1].Value()), Effort: strings.TrimSpace(p.inputs[2].Value())}}, p)
		}
		if !k.Paste && k.String() == "ctrl+l" {
			return m.auxiliaryRequest(auxiliary.Request{Action: "models", Profile: auxiliary.Profile{Account: strings.TrimSpace(p.inputs[0].Value())}}, p)
		}
		var cmd tea.Cmd
		p.inputs[p.field], cmd = p.inputs[p.field].Update(k)
		return cmd
	}
	switch k.String() {
	case "up", "down", "tab":
		p.selected = 1 - p.selected
	case "enter", "e":
		p.editing = true
		p.field = 0
		p.inputs = nil
		for _, value := range []string{profile.Account, profile.Model, profile.Effort} {
			v := textinput.New()
			v.CharLimit = 150
			v.Width = max(10, m.settingsWidth()-8)
			v.SetValue(value)
			p.inputs = append(p.inputs, v)
		}
		return p.inputs[0].Focus()
	case " ":
		profile.Enabled = !profile.Enabled
		return m.auxiliaryRequest(auxiliary.Request{Action: "put", Task: task, Profile: profile}, p)
	case "r":
		return m.auxiliaryRequest(auxiliary.Request{Action: "list"}, p)
	}
	return nil
}
func (m *model) auxiliaryScreen() string {
	p := m.settingsPage.auxiliary
	lines := []string{"", accent.Bold(true).Render("AI tasks" + m.connectionLabel(m.settingsPage.connection)), ""}
	if p.editing {
		for i, label := range []string{"Account alias", "Model", "Effort (empty: provider default)"} {
			lines = append(lines, label, p.inputs[i].View())
		}
		lines = append(lines, "Tab field · Ctrl+L list models · Ctrl+S validate and enable · Esc cancel")
	} else {
		for i, profile := range []auxiliary.Profile{p.config.Summary, p.config.Suggestion} {
			name := []string{"Summary", "Next-message suggestion"}[i]
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
	var names []string
	for _, a := range m.accounts {
		names = append(names, a.GetAlias()+" ("+a.GetAgent()+")")
	}
	lines = append(lines, "", "Accounts: "+strings.Join(names, ", "), "Local OAuth: run cxz ai login ACCOUNT on the Manager host.", "Central Codex: use cxz account login ACCOUNT.", "Checkpoint uses the summary account, or suggestion account when summary is off.", "/summary and /suggest: on/off per session, or once while off. Alt+G accepts the ghost.")
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
func auxiliaryDots(pulse int) string {
	n := pulse / 4 % 4
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
	return indentBlock(muted.Render(ansi.Hardwrap("Summary · "+safeText(text), max(1, m.view.Width-2), true))), false
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
			return "Suggestion " + auxiliaryDots(m.pulse)
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
