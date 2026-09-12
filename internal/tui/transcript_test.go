package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func TestTimestampedInput(t *testing.T) {
	stamp := time.Date(2026, 9, 12, 2, 34, 0, 0, time.Local).UnixMilli()
	got := ansi.Strip(eventView(&api.Session{}, &api.Event{Kind: "input", TimeMs: stamp, Text: "Rolem..\nIpsum..."}, 40))
	if got != "  09-12 02:34\n> Rolem..\n  Ipsum..." {
		t.Fatalf("input formatting: %q", got)
	}
	got = ansi.Strip(eventView(&api.Session{}, &api.Event{Kind: "input", TimeMs: stamp, Text: strings.Repeat("한글", 20)}, 16))
	for _, line := range strings.Split(got, "\n") {
		if ansi.StringWidth(line) > 16 {
			t.Fatalf("wrapped input overflow: %q", line)
		}
	}
}

func TestStateReplacedNotAddedToTranscript(t *testing.T) {
	m := projectModel()
	m.input = newComposer()
	m.projectView = false
	m.sessions = []*api.Session{{Id: "s", ProjectId: "p", State: "working"}}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(received{id: "s", event: &api.Event{Kind: "state", Seq: 1, Text: "working"}})
	m.Update(received{id: "s", event: &api.Event{Kind: "assistant", Seq: 2, Text: "Useful output"}})
	m.Update(received{id: "s", event: &api.Event{Kind: "state", Seq: 3, Text: "idle"}})
	m.Update(listing{sessions: []*api.Session{{Id: "s", ProjectId: "p", State: "idle"}}})
	text := ansi.Strip(m.View())
	if strings.Contains(text, "[working]") || strings.Contains(text, "[idle]") || !strings.Contains(text, "Useful output") {
		t.Fatal(text)
	}
	if len(m.events["s"]) != 3 {
		t.Fatal("state events must remain available in journal/replay data")
	}
	if strings.Contains(ansi.Strip(m.view.View()), "[idle]") {
		t.Fatal("state leaked into transcript")
	}
}

func TestEdgeToEdgeComposer(t *testing.T) {
	for _, width := range []int{40, 80, 120} {
		m := projectModel()
		m.input = newComposer()
		m.projectView = false
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		found := false
		for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
			if strings.HasPrefix(line, "╭") {
				found = true
				if ansi.StringWidth(line) != width || !strings.HasSuffix(line, "╮") {
					t.Fatalf("composer has margin: %q", line)
				}
			}
		}
		if !found {
			t.Fatal("missing composer")
		}
	}
}

func TestSpeakerPaletteAndSafeOutput(t *testing.T) {
	for _, agent := range []string{"codex", "claude", "other"} {
		s := &api.Session{Agent: agent}
		got := eventView(s, &api.Event{Kind: "assistant", Text: "hello\x1b]52;c;secret\a"}, 80)
		if strings.Contains(got, "secret") || !strings.Contains(ansi.Strip(got), "hello") {
			t.Fatal("unsafe output", got)
		}
		style := lavender.Bold(true)
		if agent == "codex" {
			style = codex
		}
		if agent == "claude" {
			style = claude
		}
		if !strings.HasPrefix(got, "  "+style.Render(strings.ToUpper(agent))+"\n") {
			t.Fatal("wrong speaker style")
		}
	}
}
