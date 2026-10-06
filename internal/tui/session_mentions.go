package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/sessionalias"
)

// Mentions remain plain text; the agent resolves the alias through session_lookup.
func mentionRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-'
}
func (m *model) mentionContext() *inlineToken {
	if m.errorDialog != nil || m.sessionArchive != nil || m.panelFocus || m.questionFocused() || m.pasteDialog != nil || m.pathHints != nil || m.terminalFocused() || m.settingsPage != nil || m.memoryPage != nil || m.library != nil {
		return nil
	}
	s := m.current()
	if s == nil || s.ProjectId == "" {
		return nil
	}
	value, pos, _, _, ok := m.chipInput()
	if !ok || strings.HasPrefix(value, "/") {
		return nil
	}
	r := []rune(value)
	if pos < 1 || pos > len(r) {
		return nil
	}
	start := pos
	for start > 0 && mentionRune(r[start-1]) {
		start--
	}
	if start == 0 || r[start-1] != '@' {
		return nil
	}
	start--
	if start > 0 && !unicode.IsSpace(r[start-1]) && !strings.ContainsRune("([{", r[start-1]) {
		return nil
	}
	if strings.Count(string(r[:start]), "`")%2 != 0 {
		return nil
	}
	end := pos
	for end < len(r) && mentionRune(r[end]) {
		end++
	}
	return &inlineToken{start: start, end: end, cursor: pos, value: value, query: string(r[start+1 : pos]), signature: fmt.Sprintf("%s:%d:%s", s.Id, pos, value)}
}
func (m *model) mentionHints() (*inlineToken, []*api.Session) {
	token := m.mentionContext()
	if token == nil || token.signature == m.mentionDismissed {
		return nil, nil
	}
	if m.mentionSignature != token.signature {
		m.mentionSignature = token.signature
		m.mentionSelected = 0
	}
	current := m.current()
	seen := map[string]bool{}
	var out []*api.Session
	// The snapshot already contains connection-scoped IDs. Never mix projects,
	// even when two managers use the same local project ID or session alias.
	for _, list := range [][]*api.Session{m.sessions, m.allSessions} {
		for _, s := range list {
			if s.Id == current.Id || s.ProjectId != current.ProjectId || seen[s.Id] || !sessionalias.Valid(s.Alias) {
				continue
			}
			seen[s.Id] = true
			if fuzzyScore(s.Alias, token.query) >= 0 {
				out = append(out, s)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := fuzzyScore(out[i].Alias, token.query), fuzzyScore(out[j].Alias, token.query)
		if a != b {
			return a < b
		}
		return out[i].Alias < out[j].Alias
	})
	m.mentionSelected = max(0, min(m.mentionSelected, len(out)-1))
	return token, out
}
func (m *model) mentionGrid(hints []*api.Session, height int) (cols, cell, rows int) {
	inner := max(1, m.width-4)
	cell = 10
	for _, s := range hints {
		cell = max(cell, len(s.Alias)+4)
	}
	cols = max(1, inner/cell)
	cell = inner / cols
	rows = min(4, max(0, height-5))
	return
}
func (m *model) mentionKey(k tea.KeyMsg) bool {
	if k.Paste || m.width < 8 || m.view.Height < 6 {
		return false
	}
	token, hints := m.mentionHints()
	if token == nil {
		return false
	}
	if k.String() == "esc" {
		m.mentionDismissed = token.signature
		return true
	}
	if len(hints) == 0 {
		return false
	}
	cols, _, rows := m.mentionGrid(hints, m.view.Height)
	switch k.String() {
	case "left":
		m.mentionSelected = max(0, m.mentionSelected-1)
	case "right":
		m.mentionSelected = min(len(hints)-1, m.mentionSelected+1)
	case "up":
		m.mentionSelected = max(0, m.mentionSelected-cols)
	case "down":
		m.mentionSelected = min(len(hints)-1, m.mentionSelected+cols)
	case "pgup":
		m.mentionSelected = max(0, m.mentionSelected-cols*rows)
	case "pgdown":
		m.mentionSelected = min(len(hints)-1, m.mentionSelected+cols*rows)
	case "enter", "tab":
		r := []rune(token.value)
		name := "@" + hints[m.mentionSelected].Alias
		if token.end == len(r) {
			name += " "
		}
		m.input.ClearSelection()
		m.setPathInput(string(r[:token.start])+name+string(r[token.end:]), token.start+len([]rune(name)))
		if next := m.mentionContext(); next != nil {
			m.mentionDismissed = next.signature
		}
	default:
		return false
	}
	return true
}
func (m *model) mentionOverlay(view string) (string, bool) {
	token, hints := m.mentionHints()
	height := len(strings.Split(view, "\n"))
	if token == nil || m.width < 8 || height < 6 {
		return view, false
	}
	cols, cell, rows := m.mentionGrid(hints, height)
	content := []string{accent.Render("Sessions · this project")}
	if len(hints) == 0 {
		content = append(content, muted.Render("No matching sessions"))
	} else {
		rows = min(rows, (len(hints)+cols-1)/cols)
		start := (m.mentionSelected / (cols * rows)) * cols * rows
		for row := 0; row < rows && start+row*cols < len(hints); row++ {
			var line strings.Builder
			for col := 0; col < cols; col++ {
				i := start + row*cols + col
				if i >= len(hints) {
					break
				}
				label := "  @" + hints[i].Alias
				style := muted
				if i == m.mentionSelected {
					label = "› @" + hints[i].Alias
					style = focus
				}
				label = ansi.Truncate(label, cell, "...")
				line.WriteString(style.Render(label + strings.Repeat(" ", max(0, cell-ansi.StringWidth(label)))))
			}
			content = append(content, line.String())
		}
		selected := hints[m.mentionSelected]
		title := strings.TrimSpace(selected.Title)
		if title == "" {
			title = "Untitled"
		}
		content = append(content, muted.Render(fmt.Sprintf("%d/%d · %s · %s", m.mentionSelected+1, len(hints), pickerLabel(title), pickerLabel(selected.Agent))))
	}
	content = append(content, muted.Render("←↑↓→ select · Enter/Tab insert · Esc close"))
	return overlayBox(view, content, m.width), true
}

func (m *model) mentionMouse(v tea.MouseMsg) bool {
	token, hints := m.mentionHints()
	height := m.view.Height
	if token == nil || m.width < 8 || height < 6 {
		return false
	}
	cols, cell, rows := m.mentionGrid(hints, height)
	start, visible := 0, 0
	if len(hints) > 0 {
		rows = min(rows, (len(hints)+cols-1)/cols)
		start = (m.mentionSelected / (cols * rows)) * cols * rows
		visible = min(rows, (len(hints)-start+cols-1)/cols)
	}
	boxHeight := visible + 5
	if len(hints) == 0 {
		boxHeight = 5
	}
	x, y := v.X-m.contentOffset(), v.Y-(height-boxHeight)
	if x < 0 || x >= m.width || y < 0 || y >= boxHeight {
		return false
	}
	if len(hints) == 0 {
		return true
	}
	if v.Button == tea.MouseButtonWheelUp {
		m.mentionSelected = max(0, m.mentionSelected-cols)
		return true
	}
	if v.Button == tea.MouseButtonWheelDown {
		m.mentionSelected = min(len(hints)-1, m.mentionSelected+cols)
		return true
	}
	col, row := (x-2)/cell, y-2
	if x < 2 || col >= cols || row < 0 || row >= visible {
		return true
	}
	i := start + row*cols + col
	if i >= len(hints) {
		return true
	}
	if v.Action == tea.MouseActionMotion {
		m.mentionSelected = i
	}
	if v.Action == tea.MouseActionPress && v.Button == tea.MouseButtonLeft {
		m.mentionSelected = i
		m.mentionKey(tea.KeyMsg{Type: tea.KeyTab})
	}
	return true
}
