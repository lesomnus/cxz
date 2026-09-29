package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/auxiliary"
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
	ctx, client := m.contextFor(connection), m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		b, _ := json.Marshal(r)
		out, e := client.Docker(ctx, &api.DockerInput{Action: "auxiliary", Spec: b})
		v := auxiliaryResult{page: p, connection: connection, session: r.Session, action: action, err: e}
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
	m.auxiliaryPolling = false
	if v.connection != m.connectionRef() {
		return
	}
	if m.auxiliaryJobs == nil {
		m.auxiliaryJobs = map[string]*auxiliary.Job{}
	}
	if v.err != nil {
		m.auxiliaryError = v.err.Error()
		return
	}
	m.auxiliaryError = ""
	old := m.auxiliaryJobs[v.connection+"/"+v.session]
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
	} else if j := v.reply.Job; j != nil && j.Status == "completed" && (old == nil || old.ID != j.ID || old.Status != "completed") {
		m.notice = "AI summary/suggestion ready · /summary"
	}

}
func (m *model) pollAuxiliary() tea.Cmd {
	if m.auxiliaryPolling || time.Since(m.auxiliaryChecked) < 5*time.Second || m.ctx == nil {
		return nil
	}
	s := m.current()
	if s == nil {
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
	if s == nil || j == nil || j.Status != "completed" || j.Run != s.RunId || s.State != "idle" || len(m.pendingInputs[s.Id]) > 0 {
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
func (m *model) showSummary() {
	text := "No auxiliary result. Enable tasks in Settings → AI tasks. Only new completed turns are processed."
	if j := m.auxiliaryJob(); j != nil {
		text = fmt.Sprintf("AI-generated · %s · turn %d\n\n%s", j.Status, j.Turn, j.Summary)
		if j.Suggestion != "" {
			text += "\n\nSuggested next message:\n" + j.Suggestion + "\n\nUse /suggest or Alt+G with an empty composer to copy."
		}
		if j.Error != "" {
			text += "\n\n" + j.Error
		}
		for _, u := range j.Usage {
			text += "\n\nAuxiliary usage · " + u.Account + " · " + u.Model + " · " + u.Task + "\n" + string(u.Data)
		}
	} else if m.auxiliaryError != "" {
		text = m.auxiliaryError
	}
	m.openReport("/summary", safeText(text))
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
			state := "Off"
			if profile.Enabled {
				state = "On"
			}
			line := fmt.Sprintf("  %-24s %s · %s · %s · %s", name, state, profile.Account, profile.Model, profile.Effort)
			if i == p.selected {
				line = focus.Render("›" + line[1:])
			}
			lines = append(lines, line)
		}
		lines = append(lines, "", "Enter edit · Space enable/disable · r refresh · Esc back")
	}
	var names []string
	for _, a := range m.accounts {
		names = append(names, a.GetAlias()+" ("+a.GetAgent()+")")
	}
	lines = append(lines, "", "Accounts: "+strings.Join(names, ", "), "Local OAuth: run cxz ai login ACCOUNT on the Manager host.", "Central Codex: use cxz account login ACCOUNT.", "Checkpoint uses the summary account, or suggestion account when summary is off.", "View /summary; Alt+G copies a suggestion into an empty composer.")
	if p.busy {
		lines = append(lines, "Checking account/model…")
	}
	lines = append(lines, strings.Split(safeText(p.message), "\n")...)
	for i := range lines {
		lines[i] = "  " + clip(lines[i], max(1, m.settingsWidth()-4))
	}
	return screen(strings.Join(lines, "\n"), m.settingsWidth(), m.height)
}
