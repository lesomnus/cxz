package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func TestFixedMetricPositions(t *testing.T) {
	for _, n := range []int{1, 10, 100, 1000, 1000000} {
		e := &api.Event{Text: "completed", Payload: []byte(fmt.Sprintf(`{"usage":{"input_tokens":%d,"output_tokens":%d},"costUSD":0.1,"duration_ms":2000}`, n, n))}
		text := ansi.Strip(turnSummary(e, nil, 0, 100))
		if !strings.HasPrefix(text, "  00:00:02") {
			t.Fatal("duration has leading padding", text)
		}
		for i, symbol := range []string{"◷", "$", "↑", "↓"} {
			offset := strings.Index(text, symbol)
			want := []int{11, 13, 23, 29}[i]
			if offset < 0 || ansi.StringWidth(text[:offset]) != want {
				t.Fatalf("symbol moved: %q", text)
			}
		}
	}
	for n, want := range map[float64]string{1: "1", 1000: "1k", 1234: "1.2k", 1000000: "1m", 999999: "1m"} {
		if got := humanCount(n); got != want {
			t.Fatalf("%v = %s, want %s", n, got, want)
		}
	}
}

func TestLocalHelpAndBottomSessionBar(t *testing.T) {
	m := projectModel()
	c := &recordingClient{}
	m.client = c
	m.projectView = false
	m.input = newComposer()
	m.sessions = []*api.Session{{Id: "s", Agent: "codex", Account: "work", Title: "Example", State: "idle"}}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if len(lines) != 30 || !strings.Contains(lines[len(lines)-1], "codex · ◉ work") {
		t.Fatal("session bar is not at bottom", strings.Join(lines, "\n"))
	}
	if strings.Contains(m.View(), "F4 interrupt") || strings.Contains(lines[0], "cxz · sessions") {
		t.Fatal("static shortcuts/header remain")
	}
	m.input.SetValue("/help")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil || len(c.inputs) != 0 || m.cursor["s"] != 0 {
		t.Fatal("help must not call agent or advance replay cursor")
	}
	m.view.GotoTop()
	if !strings.Contains(ansi.Strip(m.view.View()), "cxz /help") {
		t.Fatal("missing local help")
	}
	m.Update(received{id: "s", event: &api.Event{Seq: 1, Kind: "assistant", Text: "New reply"}})
	// Scroll to inspect the full content; help remains before subsequent replies.
	m.view.GotoTop()
	content := m.view.View()
	if !strings.Contains(content, "cxz /help") {
		t.Fatal("help lost on refresh")
	}
	m.sessions = []*api.Session{{Id: "other"}}
	m.render()
	if strings.Contains(m.view.View(), "cxz /help") {
		t.Fatal("help leaked across sessions")
	}
}
