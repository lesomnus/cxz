package tui

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
)

type accountSaved struct {
	account *resource.Account
	err     error
}
type accountLoggedIn struct{ err error }

func (m *model) openAccounts(choose bool) tea.Cmd {
	m.accountView, m.accountChoosing, m.accountLoading = true, choose, true
	m.accountAdding = false
	m.accountSearching = false
	m.accountSearch = textinput.New()
	m.accountSearch.Placeholder = "alias / name / provider / number"
	m.accountSearch.CharLimit = 100
	m.notice = "Loading accounts…"
	return m.loadAccounts()
}

func (m *model) accountChoices() []*resource.Account {
	query := strings.ToLower(strings.TrimSpace(m.accountSearch.Value()))
	var choices []*resource.Account
	for i, a := range m.accounts {
		if query == "" || query == strconv.Itoa(i+1) || strings.Contains(strings.ToLower(a.GetAlias()+" "+a.GetName()+" "+a.GetAgent()), query) {
			choices = append(choices, a)
		}
	}
	return choices
}

func (m *model) accountKey(key tea.KeyMsg) tea.Cmd {
	if key.String() == "ctrl+c" {
		return tea.Quit
	}
	if m.busy {
		return nil
	}
	if m.accountSearching {
		switch key.String() {
		case "ctrl+q":
			m.accountSearching = false
			m.accountSearch.Blur()
			return m.accountKey(key)
		case "esc":
			m.accountSearching = false
			m.accountSearch.Blur()
			return nil
		case "enter":
			m.accountSearching = false
			m.accountSearch.Blur()
			return m.accountKey(key)
		default:
			var cmd tea.Cmd
			m.accountSearch, cmd = m.accountSearch.Update(key)
			m.accountIndex = 0
			return cmd
		}
	}
	if key.String() == "esc" || key.String() == "ctrl+q" {
		if m.accountAdding {
			m.accountAdding = false
		} else {
			m.accountView = false
		}
		m.notice = ""
		return nil
	}
	if m.accountAdding {
		switch key.String() {
		case "tab", "shift+tab":
			delta := 1
			if key.String() == "shift+tab" {
				delta = 3
			}
			m.accountField = (m.accountField + delta) % 4
			m.accountAlias.Blur()
			m.accountName.Blur()
			if m.accountField == 1 {
				return m.accountAlias.Focus()
			}
			if m.accountField == 2 {
				return m.accountName.Focus()
			}
		case "left", "right", " ":
			if m.accountField == 0 {
				if m.accountAgent == "claude" {
					m.accountAgent = "codex"
				} else {
					m.accountAgent = "claude"
				}
				return nil
			}
		case "enter":
			if m.accountField != 3 {
				return m.accountKey(tea.KeyMsg{Type: tea.KeyTab})
			}
			alias, agent, name := strings.TrimSpace(m.accountAlias.Value()), m.accountAgent, strings.TrimSpace(m.accountName.Value())
			if err := accounts.Validate(alias, agent); err != nil {
				m.notice = err.Error()
				return nil
			}
			if m.accountService == nil {
				m.notice = "Account service unavailable"
				return nil
			}
			m.busy = true
			return func() tea.Msg {
				ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
				defer cancel()
				a, err := m.accountService.Add(ctx, resource.AccountAddRequest_builder{Alias: alias, Name: name, Agent: agent}.Build())
				return accountSaved{a, err}
			}
		}
		var cmd tea.Cmd
		if m.accountField == 1 {
			m.accountAlias, cmd = m.accountAlias.Update(key)
		}
		if m.accountField == 2 {
			m.accountName, cmd = m.accountName.Update(key)
		}
		return cmd
	}
	switch key.String() {
	case "/":
		m.accountSearching = true
		return m.accountSearch.Focus()
	case "n":
		m.accountAdding, m.accountField, m.accountAgent = true, 0, "claude"
		m.accountAlias, m.accountName = textinput.New(), textinput.New()
		m.accountAlias.Placeholder, m.accountName.Placeholder = "personal", "Display name (optional)"
		m.accountAlias.CharLimit, m.accountName.CharLimit = 63, 120
		m.notice = ""
	case "up":
		m.accountIndex = max(0, m.accountIndex-1)
	case "down":
		m.accountIndex = max(0, min(len(m.accountChoices())-1, m.accountIndex+1))
	case "r":
		m.accountLoading = true
		return m.loadAccounts()
	case "l", "enter":
		choices := m.accountChoices()
		if m.accountLoading || len(choices) == 0 {
			return nil
		}
		alias := choices[m.accountIndex].GetAlias()
		if key.String() == "l" {
			if m.loginAccount == nil {
				m.notice = "Login requires the workspace dashboard (cxz up)."
				return nil
			}
			m.busy = true
			e := &createProjectExec{create: func(in io.Reader, out, errOut io.Writer) (*api.Session, error) {
				return nil, m.loginAccount(alias, in, out, errOut)
			}}
			return tea.Exec(e, func(err error) tea.Msg { return accountLoggedIn{err} })
		}
		if !m.accountChoosing {
			m.notice = "Press l to log in. Esc returns to project; n adds an account."
			return nil
		}
		if m.createProjectSession == nil {
			m.notice = "Session creation unavailable"
			return nil
		}
		m.busy = true
		e := &createProjectExec{create: func(in io.Reader, out, errOut io.Writer) (*api.Session, error) {
			return m.createProjectSession(alias, in, out, errOut)
		}}
		return tea.Exec(e, func(err error) tea.Msg {
			r := result{err: err, text: "session created"}
			if err == nil && e.session != nil {
				r.sessionID = e.session.Id
			}
			return r
		})
	}
	return nil
}

func (m *model) accountScreen() string {
	width := max(1, m.width-4)
	rows := []string{brand.Render("cxz · accounts"), muted.Render("Isolated authentication profiles · Esc / Ctrl+Q returns to project"), ""}
	if m.accountAdding {
		m.accountAlias.Width, m.accountName.Width = max(1, width-16), max(1, width-16)
		fields := []string{"Provider     " + providerLabel(m.accountAgent) + "  ←/→", "Alias        " + m.accountAlias.View(), "Display name " + m.accountName.View(), "[ Create account ]"}
		for i, field := range fields {
			if i == m.accountField {
				rows = append(rows, accent.Render("› "+field))
			} else {
				rows = append(rows, "  "+field)
			}
		}
		rows = append(rows, "", muted.Render("Tab / Shift+Tab move · Enter next / create · Esc cancel"))
	} else {
		if m.accountChoosing {
			rows = append(rows, strong.Render("Choose an account for the new session"))
		}
		if len(m.accounts) == 0 && !m.accountLoading {
			rows = append(rows, peach.Render("No accounts yet. Press n to add one here."))
		}
		choices := m.accountChoices()
		capacity := max(1, (m.height-12)/2)
		start := max(0, m.accountIndex-capacity+1)
		for i := start; i < min(len(choices), start+capacity); i++ {
			a := choices[i]
			label := fmt.Sprintf("%s · %s · %s", pickerLabel(a.GetAlias()), providerLabel(a.GetAgent()), pickerLabel(a.GetName()))
			if i == m.accountIndex {
				label = accent.Render("› " + label)
			} else {
				label = "  " + label
			}
			rows = append(rows, label, muted.Render("  "+pickerLabel(a.GetAuthBackend())))
		}
		if len(choices) == 0 && len(m.accounts) > 0 {
			rows = append(rows, muted.Render("No matching accounts"))
		}
		m.accountSearch.Width = max(1, width-10)
		rows = append(rows, "", "Search: "+m.accountSearch.View(), muted.Render("↑/↓ select · n add · l login"), muted.Render("/ search · r reload · Esc back"))
		if m.accountChoosing {
			rows = append(rows, muted.Render("Enter creates session with selected account"))
		}
	}
	status := m.notice
	if m.busy {
		status = "Working… " + status
	} else if m.accountLoading {
		status = "Loading accounts…"
	}
	rows = append(rows, "", warning.Render(pickerLabel(status)))
	return screen(indentBlock(strings.Join(rows, "\n")), m.width, m.height)
}
