package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"strings"
	"testing"
)

func TestCommandOverlayAndPlaceholders(t *testing.T) {
	m := projectModel()
	c := &recordingClient{}
	m.client = c
	m.projectView = false
	m.input = newComposer()
	m.sessions = []*api.Session{{Id: "s", Agent: "claude"}}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	before := m.view.View()
	offset := m.view.YOffset
	m.input.SetValue("/")
	screen := ansi.Strip(m.View())
	if lipgloss.ColorProfile().Name() == "Ascii" && strings.Contains(m.View(), "\x1b") {
		t.Fatal("ANSI escapes in uncolored output")
	}
	if !strings.Contains(screen, "/context") || !strings.Contains(screen, "/compact") || !strings.Contains(screen, "/usage") {
		t.Fatal(screen)
	}
	if m.view.View() != before || m.view.YOffset != offset || len(strings.Split(screen, "\n")) != 24 {
		t.Fatal("overlay changed transcript/layout")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.input.Value() != "/context" || m.focusList {
		t.Fatal("hint completion failed")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.commandHints()) != 0 || m.input.Value() != "/context" {
		t.Fatal("escape must only dismiss hints")
	}
	for _, cmd := range []string{"/context", "/compact", "/usage", "/context details"} {
		m.input.SetValue(cmd)
		_, action := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if action != nil || len(c.inputs) != 0 || m.cursor["s"] != 0 {
			t.Fatal("placeholder was sent to agent")
		}
		if !strings.Contains(ansi.Strip(m.view.View()), "Unimplemented") {
			t.Fatal("missing placeholder reply")
		}
	}
	m.input.SetValue("ordinary input")
	if len(m.commandHints()) != 0 {
		t.Fatal("hints for ordinary input")
	}
}

func TestTranscriptGutterAndDuration(t *testing.T) {
	for _, kind := range []string{"assistant", "tool_call", "tool_result", "diagnostic", "approval", "approval_resolved"} {
		text := ansi.Strip(eventView(&api.Session{Agent: "codex"}, &api.Event{Kind: kind, Text: "hello", Payload: []byte(`{"value":"world"}`)}, 40))
		for _, line := range strings.Split(text, "\n") {
			if !strings.HasPrefix(line, "  ") || ansi.StringWidth(line) > 40 {
				t.Fatal(kind, line)
			}
		}
	}
	for ms, want := range map[float64]string{0: "00:00:00 ◷", 2000: "00:00:02 ◷", 62000: "00:01:02 ◷", 3661000: "01:01:01 ◷"} {
		if got := ansi.Strip(clockMetric(ms, false)); got != want {
			t.Fatalf("clock: %q", got)
		}
	}
	if got := clockMetric(2000, false); !strings.Contains(got, zeroStyle.Render("00")) {
		t.Fatal("zero units not dimmed")
	}
	if got := black.GetBackground(); got == nil {
		t.Fatal("missing background")
	}
}
