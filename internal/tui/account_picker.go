package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/resource"
)

var ErrAccountSelectionCanceled = errors.New("account selection canceled")

// SelectAccount uses the caller's terminal and keeps stdout free for CLI JSON.
// The alias is returned only after an explicit Enter on a visible candidate.
func SelectAccount(ctx context.Context, accounts []*resource.Account, input io.Reader, output io.Writer) (string, error) {
	if len(accounts) == 0 {
		return "", errors.New("no accounts to select")
	}
	m := newAccountPicker(accounts)
	_, err := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen()).Run()
	if err != nil {
		return "", fmt.Errorf("account selection: %w", err)
	}
	if m.chosen == "" {
		return "", ErrAccountSelectionCanceled
	}
	return m.chosen, nil
}

type accountPicker struct {
	accounts      []*resource.Account
	matches       []int
	selected      int
	search        textinput.Model
	width, height int
	chosen        string
	finished      bool
}

func newAccountPicker(accounts []*resource.Account) *accountPicker {
	s := textinput.New()
	s.Prompt = "Search: "
	s.Placeholder = "number, name, alias (or agent)"
	s.CharLimit = 200
	s.Focus()
	m := &accountPicker{accounts: accounts, search: s, width: 80, height: 24}
	m.filter()
	return m
}

func (m *accountPicker) Init() tea.Cmd { return textinput.Blink }

func (m *accountPicker) filter() {
	m.matches = nil
	m.selected = 0
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	// Exact number/name/alias matches take priority, but retain all collisions.
	// Numbers refer to the original list, never to renumbered search results.
	if q != "" {
		for i, a := range m.accounts {
			if q == strconv.Itoa(i+1) || q == strings.ToLower(a.GetAlias()) || q == strings.ToLower(a.GetName()) {
				m.matches = append(m.matches, i)
			}
		}
		if len(m.matches) > 0 {
			return
		}
	}
	for i, a := range m.accounts {
		haystack := strings.ToLower(a.GetAlias() + " " + a.GetName() + " " + a.GetAgent())
		match := true
		for _, word := range strings.Fields(q) {
			if !strings.Contains(haystack, word) {
				match = false
				break
			}
		}
		if match {
			m.matches = append(m.matches, i)
		}
	}
}

func (m *accountPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.finished {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.Width = max(1, msg.Width-12)
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.finished = true
			return m, tea.Quit
		case "up", "ctrl+p":
			m.selected = max(0, m.selected-1)
			return m, nil
		case "down", "ctrl+n":
			m.selected = max(0, min(len(m.matches)-1, m.selected+1))
			return m, nil
		case "enter":
			if len(m.matches) > 0 {
				m.chosen = m.accounts[m.matches[m.selected]].GetAlias()
				m.finished = true
				return m, tea.Quit
			}
			return m, nil
		}
	}
	previous := m.search.Value()
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	if previous != m.search.Value() {
		m.filter()
	}
	return m, cmd
}

func pickerLabel(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}

func (m *accountPicker) View() string {
	if m.finished {
		return ""
	}
	rows := max(1, m.height-6)
	start := max(0, m.selected-rows+1)
	var lines []string
	lines = append(lines, fmt.Sprintf("Select Account · %d/%d matches", len(m.matches), len(m.accounts)), "")
	for row := 0; row < rows; row++ {
		i := start + row
		line := ""
		if i < len(m.matches) {
			index := m.matches[i]
			a := m.accounts[index]
			cursor := "  "
			if i == m.selected {
				cursor = "> "
			}
			line = fmt.Sprintf("%s%d) %s · %s · %s", cursor, index+1, pickerLabel(a.GetAlias()), pickerLabel(a.GetAgent()), pickerLabel(a.GetName()))
		} else if row == 0 {
			line = "No matching accounts. Edit or clear the search."
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", m.search.View(), "↑/↓ move · Enter select · Esc/Ctrl-C cancel")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, max(1, m.width), "")
	}
	return strings.Join(lines, "\n")
}
