package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type slashCommand struct{ name, description string }

var slashCommands = []slashCommand{
	{"/help", "Categories, shortcuts and command examples"},
	{"/context", "Inspect current context"},
	{"/compact", "Compact agent context"},
	{"/usage", "Session tokens, cost and time"},
	{"/answer", "Reply to a pending question"},
	{"/stop", "Stop agent"},
	{"/restart", "Restart this session's agent (confirmation required)"},
	{"/permission", "full: auto-approve this run · ask: manual"},
	{"/approval", "Inspect selected approval payload"},
	{"/details", "Inspect latest tool result"},
	{"/model", "Provider model catalog or model selection"},
	{"/effort", "Provider reasoning strength"},
}

func (m *model) commandHints() []slashCommand {
	text := m.input.Value()
	if m.creating || m.focusList || m.focusApproval || m.hintDismissed || !strings.HasPrefix(text, "/") || strings.ContainsAny(text, " \n\t") {
		return nil
	}
	var matches []slashCommand
	for _, c := range slashCommands {
		if fuzzyScore(c.name, text) >= 0 {
			matches = append(matches, c)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return fuzzyScore(matches[i].name, text) < fuzzyScore(matches[j].name, text) })
	return matches
}

// Subsequence matching, preferring exact/prefix and tightly clustered matches.
func fuzzyScore(name, query string) int {
	name, query = strings.ToLower(name), strings.ToLower(query)
	if name == query {
		return 0
	}
	if strings.HasPrefix(name, query) {
		return 1
	}
	pos, score := 0, 2
	for _, r := range query {
		i := strings.IndexRune(name[pos:], r)
		if i < 0 {
			return -1
		}
		score += i
		pos += i + len(string(r))
	}
	return score
}

func (m *model) commandKey(k tea.KeyMsg) (bool, tea.Cmd) {
	hints := m.commandHints()
	if len(hints) > 0 {
		m.hintSelected = max(0, min(m.hintSelected, len(hints)-1))
		switch k.String() {
		case "up":
			m.hintSelected = (m.hintSelected + len(hints) - 1) % len(hints)
			return true, nil
		case "down":
			m.hintSelected = (m.hintSelected + 1) % len(hints)
			return true, nil
		case "esc":
			m.hintDismissed = true
			return true, nil
		case "tab":
			m.input.SetValue(hints[m.hintSelected].name)
			m.hintSelected = 0
			return true, nil
		case "ctrl+s":
			m.input.SetValue(hints[m.hintSelected].name)
			m.hintSelected = 0
			// Submission stays in the ordinary send handler.
			return false, nil
		}
	}
	if k.Type == tea.KeyRunes || k.String() == "backspace" || k.String() == "ctrl+x" {
		m.hintDismissed = false
		m.hintSelected = 0
	}
	return false, nil
}

// Replace the bottom visible transcript rows, without touching scroll position
// or saved events. The composer and status line never move for this overlay.
func (m *model) commandOverlay(view string) string {
	hints := m.commandHints()
	if len(hints) == 0 {
		return view
	}
	rows := strings.Split(view, "\n")
	count := min(7, min(len(hints), max(0, len(rows)-2)))
	if count == 0 {
		return view
	}
	selected := max(0, min(m.hintSelected, len(hints)-1))
	margin := min(2, (count-1)/2)
	start := min(m.hintOffset, selected-margin)
	start = max(start, selected+margin-count+1)
	start = max(0, min(start, len(hints)-count))
	m.hintOffset = start
	var content []string
	for i := 0; i < count; i++ {
		c := hints[start+i]
		text := "  " + c.name + "  " + c.description
		style := muted
		if start+i == selected {
			text = "› " + c.name + "  " + c.description
			style = accent
		}
		content = append(content, style.Render(text))
	}
	return overlayBox(view, content, m.width)
}

func (m *model) localCommandView(id string) (out string) {
	defer func() { out = insetRule(m.view.Width, muted) + "\n" + out }()
	command := m.localOutput[id]
	if command == "/model" {
		return localReport(m.modelReport(), m.view.Width)
	}
	if command == "/model" || command == "/permission" || command == "/approval" || command == "/context" || command == "/compact" || command == "/details" {
		return localReport(m.localReports[id], m.view.Width)
	}
	if command == "/usage" {
		return indentBlock(ansi.Hardwrap(safeText(m.usageReports[id]), max(1, m.view.Width-2), true))
	}
	if command == "" || strings.Fields(command)[0] == "/help" {
		return helpView(m.view.Width, strings.TrimSpace(strings.TrimPrefix(command, "/help")))
	}
	return indentBlock(peach.Render(command + " · Unimplemented"))
}
