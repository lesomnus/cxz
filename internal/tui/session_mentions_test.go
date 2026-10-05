package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"strings"
	"testing"
)

func mentionModel() *model {
	m := conversationModel()
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	m.current().ProjectId = "work::p"
	m.current().Alias = "self"
	m.allSessions = []*api.Session{m.current(), {Id: "work::a", ProjectId: "work::p", Alias: "seal", Title: "API design", Agent: "codex"}, {Id: "work::b", ProjectId: "work::p", Alias: "cedar", Agent: "claude"}, {Id: "home::c", ProjectId: "home::p", Alias: "foreign"}, {Id: "work::d", ProjectId: "work::other", Alias: "other"}}
	return m
}
func TestMentionScopeAndBoundaries(t *testing.T) {
	m := mentionModel()
	for _, value := range []string{"@", "look @", "한글\n@", "(@", "@SE"} {
		m.setPathInput(value, len([]rune(value)))
		token, hints := m.mentionHints()
		if token == nil || len(hints) == 0 {
			t.Fatal("missing", value)
		}
		for _, s := range hints {
			if s.Id == m.current().Id || s.ProjectId != m.current().ProjectId {
				t.Fatal("wrong scope", s)
			}
		}
	}
	for _, value := range []string{"mail@seal", "https://host/@seal", "/@redact", "`@seal", "`code @seal", "foo/@", "@@", "/download @"} {
		m.setPathInput(value, len([]rune(value)))
		if token, _ := m.mentionHints(); token != nil {
			t.Fatal("false trigger", value)
		}
	}
	m.setPathInput("@", 1)
	_, hints := m.mentionHints()
	if len(hints) != 2 {
		t.Fatal("wrong candidates", hints)
	}
}
func TestMentionInsertPreservesUnicodeSuffixAndEsc(t *testing.T) {
	m := mentionModel()
	value := "첫 줄\n참고 @se 뒤 문장"
	m.setPathInput(value, len([]rune("첫 줄\n참고 @se")))
	before := m.view.YOffset
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.input.Value() != "첫 줄\n참고 @seal 뒤 문장" || m.view.YOffset != before {
		t.Fatal(m.input.Value())
	}
	_, pos, _, _, _ := m.chipInput()
	if pos != len([]rune("첫 줄\n참고 @seal")) {
		t.Fatal("cursor", pos)
	}
	if token, _ := m.mentionHints(); token != nil {
		t.Fatal("reopened after acceptance")
	}
	m.setPathInput("@", 1)
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if token, _ := m.mentionHints(); token != nil || m.input.Value() != "@" {
		t.Fatal("dismiss")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if token, _ := m.mentionHints(); token == nil {
		t.Fatal("did not reopen after editing")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.input.Value() != "@seal " {
		t.Fatal(m.input.Value())
	}
}
func TestMentionGridPagingResizeAndMouse(t *testing.T) {
	m := mentionModel()
	for i := 0; i < 70; i++ {
		m.allSessions = append(m.allSessions, &api.Session{Id: fmt.Sprint(i), ProjectId: "work::p", Alias: fmt.Sprintf("item-%02d", i)})
	}
	m.setPathInput("@", 1)
	_, hints := m.mentionHints()
	cols, _, _ := m.mentionGrid(hints, m.view.Height)
	if cols < 2 {
		t.Fatal("expected multiple columns")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.mentionSelected != 1 {
		t.Fatal("right")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.mentionSelected != 1+cols {
		t.Fatal("down")
	}
	for i := 0; i < 20; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	}
	if m.mentionSelected != len(hints)-1 {
		t.Fatal("last page")
	}
	view := m.commandOverlay(strings.Repeat("line\n", m.view.Height-1) + "line")
	if !strings.Contains(ansi.Strip(view), "@seal") {
		t.Fatal("selected not visible")
	}
	m.Update(tea.WindowSizeMsg{Width: 45, Height: 18})
	view = m.commandOverlay(strings.Repeat("line\n", m.view.Height-1) + "line")
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatal("overflow")
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	m.mentionSelected = 0
	_, cell, rows := m.mentionGrid(hints, m.view.Height)
	rows = min(rows, (len(hints)+cols-1)/cols)
	m.Update(tea.MouseMsg{X: m.contentOffset() + 2 + cell, Y: m.view.Height - (rows + 5) + 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.input.Value() != "@"+hints[1].Alias+" " {
		t.Fatal("mouse selection", m.input.Value())
	}
}
func TestMentionPasteAndEmptyMatches(t *testing.T) {
	m := mentionModel()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@se"), Paste: true})
	if token, _ := m.mentionHints(); token != nil {
		t.Fatal("paste opened picker")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.input.Value() != "@se\n" {
		t.Fatal("paste Enter completed alias")
	}
	m.setPathInput("@zzzz", 5)
	text := ansi.Strip(m.commandOverlay(strings.Repeat("line\n", 12)))
	if !strings.Contains(text, "No matching sessions") {
		t.Fatal(text)
	}
}
