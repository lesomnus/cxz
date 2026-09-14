package tui

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

var inlineCommands = []slashCommand{{"@redact", "Insert a secret chip · Enter select · Tab complete · Esc dismiss"}}

type inlineToken struct {
	start, end, cursor      int
	value, query, signature string
}

func (m *model) inlineContext() *inlineToken {
	if m.workflow != nil || m.questionDialog != nil || m.pasteDialog != nil || m.pathHints != nil || m.panelFocus {
		return nil
	}
	value, pos, _, _, ok := m.chipInput()
	if !ok {
		return nil
	}
	r := []rune(value)
	if pos < 0 || pos > len(r) {
		return nil
	}
	start := pos
	for start > 0 && !unicode.IsSpace(r[start-1]) {
		start--
	}
	if start >= len(r) || r[start] != '@' || pos <= start {
		return nil
	}
	end := pos
	for end < len(r) && !unicode.IsSpace(r[end]) {
		end++
	}
	for _, c := range r[start+1 : end] {
		if c < 'a' || c > 'z' {
			return nil
		}
	}
	// Backtick literals (including path hints) are never inline commands.
	if strings.Count(string(r[:start]), "`")%2 != 0 {
		return nil
	}
	return &inlineToken{start: start, end: end, cursor: pos, value: value, query: string(r[start:pos]), signature: fmt.Sprintf("%d:%s", pos, value)}
}
func (m *model) inlineHints() (*inlineToken, []slashCommand) {
	token := m.inlineContext()
	if token == nil || token.signature == m.inlineDismissed {
		return nil, nil
	}
	var hints []slashCommand
	for _, c := range inlineCommands {
		if fuzzyScore(c.name, token.query) >= 0 {
			hints = append(hints, c)
		}
	}
	return token, hints
}
func (m *model) inlineKey(k tea.KeyMsg) bool {
	if k.Paste {
		return false
	}
	token, hints := m.inlineHints()
	if len(hints) == 0 {
		return false
	}
	switch k.String() {
	case "up", "down":
		return true
	case "esc":
		m.inlineDismissed = token.signature
		return true
	case "tab":
		r := []rune(token.value)
		name := hints[0].name
		m.setPathInput(string(r[:token.start])+name+string(r[token.end:]), token.start+len([]rune(name)))
		return true
	case "enter":
		m.openRedact()
		if d := m.redactDialog; d != nil {
			d.start, d.end = token.start, token.end
		}
		return true
	}
	return false
}
