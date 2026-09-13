package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

func TestHelpTopicsAndKeycaps(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	a, b := helpKeycaps("Ctrl+S", 0), helpKeycaps("Ctrl+S", 1)
	if ansi.Strip(a) != " Ctrl  +  S " || ansi.Strip(b) != ansi.Strip(a) || a == b {
		t.Fatalf("key padding or alternating background missing: %q / %q", a, b)
	}
	if !strings.Contains(a, "48;2;8;8;8") || !strings.Contains(b, "48;2;24;24;24") {
		t.Fatal("chord separator has no distinct truecolor background")
	}
	lipgloss.SetColorProfile(termenv.ANSI256)
	if helpKeycaps("Ctrl+S", 0) != helpKeycaps("Ctrl+S", 1) {
		t.Fatal("256-color backgrounds should both be black")
	}
	lipgloss.SetColorProfile(termenv.ANSI)
	if helpKeycaps("Ctrl+S", 0) != helpKeycaps("Ctrl+S", 1) {
		t.Fatal("low-color keycaps must share black background")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	for _, width := range []int{20, 80, 120} {
		for _, entry := range commandHelpEntries {
			for _, line := range strings.Split(helpView(width, entry.name), "\n") {
				if ansi.StringWidth(line) > width {
					t.Fatalf("help overflows width %d: %q", width, line)
				}
			}
			text := ansi.Strip(helpView(160, entry.name))
			if !strings.Contains(text, strings.ReplaceAll(entry.example, "\n", "\n  ")) || !strings.Contains(text, "Examples") {
				t.Fatal("missing command example", entry.name)
			}
		}
	}
	for _, command := range slashCommands {
		if strings.Contains(helpView(100, strings.TrimPrefix(command.name, "/")), "Unknown help topic") {
			t.Fatal("undocumented command", command.name)
		}
	}
	if !strings.Contains(helpView(100, "missing"), "Unknown help topic") {
		t.Fatal("missing unknown-topic feedback")
	}
}

func TestHelpAnswerIsLocalAndKeepsTopic(t *testing.T) {
	m := projectModel()
	c := &recordingClient{}
	m.client, m.projectView = c, false
	m.input = newComposer()
	m.sessions = []*api.Session{{Id: "s", Agent: "claude", State: "idle"}}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.input.SetValue("/help answer")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil || len(c.inputs) != 0 || m.localOutput["s"] != "/help answer" {
		t.Fatal("help topic lost or sent to agent")
	}
	if !strings.Contains(ansi.Strip(m.localCommandView("s")), `/answer {"question-id":"Use SQLite"}`) {
		t.Fatal("answer example not rendered")
	}
}
