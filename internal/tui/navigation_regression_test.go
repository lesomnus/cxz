package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/lesomnus/cxz/api"
)

func TestSettingsKeyDoesNotCapturePastePreview(t *testing.T) {
	m := conversationModel()
	m.capturePaste(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat("paste", 300)), Paste: true})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.settingsPage != nil || m.pasteDialog == nil {
		t.Fatal("Ctrl+P did not open the paste preview")
	}
	for _, code := range []rune{',', '<'} {
		key, ok := terminalEventKey(uv.KeyPressEvent{Code: code, Mod: uv.ModCtrl | uv.ModShift})
		if !ok || key.Type != tea.KeyF19 {
			t.Fatal("settings key lost its modifiers", key)
		}
	}
}

func TestQuestionCanYieldFocusWithoutLosingAnswer(t *testing.T) {
	m := questionModel()
	m.syncQuestion()
	d := m.questionDialog
	d.selected[0][1] = true
	d.other[0].SetValue("draft answer")
	m.Update(tea.KeyMsg{Type: tea.KeyF6})
	if m.questionFocused() || m.questionDialog != d || !m.input.Focused() {
		t.Fatal("question trapped keyboard focus")
	}
	m.Update(questionKeyMsg("composer draft"))
	if m.input.Value() != "composer draft" || d.other[0].Value() != "draft answer" {
		t.Fatal("input went to the unfocused question")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF6})
	if !m.questionFocused() || !d.selected[0][1] {
		t.Fatal("question answer was lost")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if !m.panelFocus || m.questionDialog != d || m.questionFocused() {
		t.Fatal("cannot inspect other sessions")
	}
}

func TestConversationWheelTakesFocus(t *testing.T) {
	for _, focus := range []string{"panel", "question", "terminal"} {
		m := questionModel()
		if focus == "question" {
			m.syncQuestion()
		}
		if focus == "panel" {
			m.panelFocus = true
		}
		if focus == "terminal" {
			m.terminals = map[string]*terminalPanel{"s": {open: true, focused: true}}
		}
		m.resize()
		m.view.SetContent(strings.Repeat("conversation line\n", 100))
		m.view.GotoBottom()
		before := m.view.YOffset
		m.Update(tea.MouseMsg{X: m.contentOffset() + 3, Y: 1, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
		if m.panelFocus || m.questionFocused() || m.terminalFocused() || m.view.YOffset >= before {
			t.Fatal("wheel failed to focus and scroll", focus)
		}
	}
}

func TestQuestionDraftSurvivesMouseNavigationAndTerminalFocus(t *testing.T) {
	m := questionModel()
	original := m.current()
	other := &api.Session{Id: "other", ProjectId: "p", Agent: "claude", RunId: "other-run"}
	m.allSessions = []*api.Session{original, other}
	m.sessions = m.allSessions
	m.syncQuestion()
	d := m.questionDialog
	d.selected[0][1] = true
	d.other[0].SetValue("keep this answer")
	rows := m.panelRows()
	var otherY int
	for y, index := range panelLayout(rows) {
		if index >= 0 && rows[index].session == other {
			otherY = projectPanelHeaderRows + y
		}
	}
	if otherY == 0 {
		t.Fatal("missing other session in panel")
	}
	m.Update(tea.MouseMsg{X: 3, Y: otherY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.current() != other || m.questionDialog != nil {
		t.Fatal("question prevented clicking another session")
	}
	m.selectPanelProject(panelRow{project: m.project, session: original})
	m.projectView, m.panelFocus = false, false
	m.syncQuestion()
	if m.questionDialog != d || m.questionFocused() || !d.selected[0][1] || d.other[0].Value() != "keep this answer" {
		t.Fatal("returning to the session lost the suspended question draft")
	}
	m.terminals = map[string]*terminalPanel{original.Id: {open: true, focused: true}}
	m.Update(tea.KeyMsg{Type: tea.KeyF6})
	if !m.questionFocused() || m.terminalFocused() || m.input.Focused() {
		t.Fatal("F6 did not transfer focus from the terminal back to the question")
	}
}

func TestWrappedConversationLinksKeepTheirDestination(t *testing.T) {
	link := "https://github.com/lesomnus/cr/issues/30#issuecomment-5847534119"
	for _, text := range []string{"이슈 #30: " + link, "- **Comment** [issue](" + link + ")", "> " + link} {
		for _, width := range []int{24, 50, 80} {
			view := markdownView(text, width)
			linkedRows := 0
			for _, row := range strings.Split(view, "\n") {
				if !strings.Contains(row, ansi.SetHyperlink(link)) {
					continue
				}
				linkedRows++
				term := vt.NewEmulator(width, 2)
				term.WriteString(row)
				found := false
				for x := 0; x < width; x++ {
					if c := term.CellAt(x, 0); c != nil && c.Link.URL == link {
						found = true
					}
				}
				term.Close()
				if !found {
					t.Fatal("viewport row lost its full hyperlink destination", row)
				}
			}
			if linkedRows == 0 {
				t.Fatal("missing hyperlink", text, view)
			}
		}
	}
	if strings.Contains(markdownView("[bad](javascript:alert)", 60), "\x1b]8;") {
		t.Fatal("unsupported scheme emitted")
	}
}
