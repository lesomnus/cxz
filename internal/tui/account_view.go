package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
)

type accountSaved struct {
	account *resource.Account
	err     error
}
type accountLoggedIn struct{ err error }

func (m *model) openAccounts(choose bool) tea.Cmd {
	m.accountConnection = m.connectionRef()
	m.accounts = nil
	m.accountIndex = 0
	m.accountView, m.accountChoosing, m.accountLoading = true, choose, true
	m.accountAdding = false
	m.loginChoosing = false
	m.accountSearching = false
	m.accountSearch = textinput.New()
	m.accountSearch.Cursor.Style = inputCursorStyle
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
	if m.loginChoosing {
		return m.loginTargetKey(key)
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
			m.panelFocus = m.projectView
		}
		m.notice = ""
		return nil
	}
	if m.accountAdding {
		switch key.String() {
		case "tab", "shift+tab", "up", "down":
			delta := 1
			if key.String() == "shift+tab" || key.String() == "up" {
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
			service := m.accountClient()
			if service == nil {
				m.notice = "Account service unavailable"
				return nil
			}
			m.busy = true
			return func() tea.Msg {
				ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
				defer cancel()
				a, err := service.Add(ctx, resource.AccountAddRequest_builder{Alias: alias, Name: name, Agent: agent}.Build())
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
		m.accountAlias.Cursor.Style, m.accountName.Cursor.Style = inputCursorStyle, inputCursorStyle
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
				m.notice = "Log in on the daemon host with cxz up, then reconnect here."
				return nil
			}
			if choices[m.accountIndex].GetAgent() == "claude" {
				m.loginChoosing = true
				m.loginAlias = alias
				m.loginIndex = 0
				m.notice = "Claude login is isolated per session."
				return nil
			}
			return m.startAccountWorkflow(alias, choices[m.accountIndex].GetAgent(), "", false)
		}
		if !m.accountChoosing {
			m.notice = "Press l to log in. Esc returns to Projects; n adds an account."
			return nil
		}
		if m.createProjectSession == nil {
			m.notice = "Session creation unavailable"
			return nil
		}
		return m.startAccountWorkflow(alias, choices[m.accountIndex].GetAgent(), "", true)
	}
	return nil
}

func (m *model) accountScreen() string {
	width := max(1, m.width-4)
	rows := []string{brand.Render("cxz · accounts" + m.connectionLabel(m.accountConnection)), muted.Render("Isolated authentication profiles · Esc / Ctrl+Q returns to project"), ""}
	if m.loginChoosing {
		rows = append(rows, strong.Render("Choose a session to log in · "+pickerLabel(m.loginAlias)))
		sessions := m.loginSessions()
		start := max(0, m.loginIndex-max(1, m.height-10)+1)
		for i := start; i <= len(sessions) && i < start+max(1, m.height-10); i++ {
			label := "[ New session + independent login ]"
			if i < len(sessions) {
				label = pickerLabel(sessions[i].Alias) + " · " + pickerLabel(sessions[i].State)
			}
			if i == m.loginIndex {
				label = accent.Render("› " + label)
			} else {
				label = "  " + label
			}
			rows = append(rows, label)
		}
		rows = append(rows, "", muted.Render("↑/↓ Tab select · Enter continue · Esc back"))
	} else if m.accountAdding {
		m.accountAlias.Width, m.accountName.Width = max(1, width-16), max(1, width-16)
		fields := []string{"Provider     " + providerLabel(m.accountAgent) + "  ←/→", "Alias        " + m.accountAlias.View(), "Display name " + m.accountName.View(), "[ Create account ]"}
		for i, field := range fields {
			if i == m.accountField {
				rows = append(rows, accent.Render("› "+field))
			} else {
				rows = append(rows, "  "+field)
			}
		}
		rows = append(rows, "", muted.Render("↑/↓ or Tab / Shift+Tab move · Enter next / create · Esc cancel"))
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
