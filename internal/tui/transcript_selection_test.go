package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"testing"
)

func TestTranscriptWord(t *testing.T) {
	for _, tc := range []struct {
		row  string
		x    int
		want string
	}{
		{"  hello_world next", 5, "hello_world"},
		{"  한글 next", 3, "한글"},
		{"  e\u0301cho next", 2, "e\u0301cho"},
		{"  👩‍💻 next", 3, "👩‍💻"},
		{"  alpha, beta", 7, ","},
	} {
		a, b := transcriptWord(tc.row, tc.x)
		if got := ansi.Cut(tc.row, a, b); got != tc.want {
			t.Fatalf("%q at %d: %q", tc.row, tc.x, got)
		}
	}
}

func TestTranscriptDoubleClickKeepsWordOnRelease(t *testing.T) {
	m := conversationModel()
	m.events["s"] = []*api.Event{{Kind: "assistant", Seq: 1, Text: "hello 한글 world"}}
	m.render()
	m.view.GotoTop()
	for i := 0; i < 2; i++ {
		m.Update(tea.MouseMsg{X: m.contentOffset() + 9, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		m.Update(tea.MouseMsg{X: m.contentOffset() + 9, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	}
	if got := m.textSelection.text(); got != "한글" {
		t.Fatalf("double click: %q", got)
	}
}

func TestTranscriptDragExcludesOnlyResponseIndent(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		m := conversationModel()
		m.events["s"] = []*api.Event{{Kind: "assistant", Seq: 1, Text: "alpha\n  indented"}}
		m.render()
		m.view.GotoTop()
		x1, y1, x2, y2 := 0, 1, 100, 2
		if reverse {
			x1, y1, x2, y2 = x2, y2, x1, y1
		}
		m.beginSelection(tea.MouseMsg{X: x1, Y: y1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		m.selectionMouse(tea.MouseMsg{X: m.contentOffset() + x2, Y: y2, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
		if got := m.textSelection.text(); got != "alpha\n  indented" {
			t.Fatalf("reverse=%v: %q", reverse, got)
		}
	}
}
