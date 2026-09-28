package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestBottomButtonReturnsToLatest(t *testing.T) {
	m := conversationModel()
	m.view.SetContent(strings.Repeat("conversation line\n", 100))
	m.view.SetYOffset(10)
	m.input.SetValue("unsent draft")
	x, y, label := m.bottomButton()
	rows := strings.Split(ansi.Strip(m.sessionScreen()), "\n")
	if label == "" || !strings.Contains(rows[y], label) || !strings.Contains(rows[y+1], "◆︎") || len(rows) != m.height {
		t.Fatal("button must sit immediately above the scroll track without changing layout")
	}
	m.Update(tea.MouseMsg{X: m.contentOffset() + x, Y: y, Action: tea.MouseActionMotion})
	if !m.bottomButtonHover {
		t.Fatal("missing hover")
	}
	m.Update(tea.MouseMsg{X: m.contentOffset() + x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.view.AtBottom() || m.input.Value() != "unsent draft" || m.textSelection != nil || m.report != nil {
		t.Fatal("click failed to scroll or interfered with other interactions")
	}
	if _, _, label := m.bottomButton(); label != "" {
		t.Fatal("button remains at latest")
	}
}

func TestBottomButtonWheelAndModal(t *testing.T) {
	m := conversationModel()
	m.view.SetContent(strings.Repeat("line\n", 100))
	m.view.SetYOffset(10)
	x, y, _ := m.bottomButton()
	m.Update(tea.MouseMsg{X: m.contentOffset() + x, Y: y, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if m.view.YOffset >= 10 || m.view.AtBottom() {
		t.Fatal("button intercepted wheel scrolling")
	}
	m.openReport("/context", "report")
	if _, _, label := m.bottomButton(); label != "" {
		t.Fatal("button visible over report")
	}
}

func TestBottomButtonLoadsDetachedTail(t *testing.T) {
	m := conversationModel()
	m.view.SetContent(strings.Repeat("old line\n", 100))
	m.view.GotoBottom()
	w := m.historyWindow(m.current().Id)
	w.detached = true
	m.current().LastSeq = 1000
	x, y, label := m.bottomButton()
	if label == "" {
		t.Fatal("loaded bottom is not the journal tail")
	}
	_, cmd := m.Update(tea.MouseMsg{X: m.contentOffset() + x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd == nil || !w.loading || w.direction != 1 || w.generation == 0 {
		t.Fatal("click must request the latest history page")
	}
}

func TestBottomButtonNarrowWidth(t *testing.T) {
	m := conversationModel()
	m.view.SetContent(strings.Repeat("line\n", 100))
	for _, width := range []int{1, 3, 8, 20, 80} {
		m.width = width
		m.view.Width = width
		x, _, label := m.bottomButton()
		if x < 0 || x+ansi.StringWidth(label) > width {
			t.Fatal("button extends beyond conversation")
		}
		for _, row := range strings.Split(m.conversationView(), "\n") {
			if ansi.StringWidth(row) > width {
				t.Fatal("button expanded conversation width")
			}
		}
	}
}
