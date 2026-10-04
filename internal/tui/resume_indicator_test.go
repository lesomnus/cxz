package tui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"testing"
)

func TestResumeIndicatorLifecycle(t *testing.T) {
	for _, failure := range []error{nil, errors.New("resume failed")} {
		m := panelModel()
		s := m.current()
		m.client = &restartClient{run: s.RunId, resumeErr: failure}
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
		if cmd == nil || !m.resumePending[s.Id] {
			t.Fatal("resume not tracked")
		}
		m.pulse = 0
		if ansi.Strip(m.sessionIndicator(s)) != "●" {
			t.Fatal("missing dot")
		}
		m.pulse = 5
		if ansi.Strip(m.sessionIndicator(s)) != " " {
			t.Fatal("dot does not blink")
		}
		if m.action("resume", "") != nil {
			t.Fatal("duplicate resume submitted")
		}
		m.Update(result{text: "unrelated"})
		if !m.resumePending[s.Id] {
			t.Fatal("unrelated action cleared resume")
		}
		result := cmd()
		m.panelIndex = 0
		m.focusPanel()
		m.Update(result)
		if m.resumePending[s.Id] {
			t.Fatal("completed resume indicator retained")
		}
	}
}
