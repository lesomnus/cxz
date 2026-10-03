package tui

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/ansisvg"
	"github.com/muesli/termenv"
)

var writeFigures = flag.Bool("figures", false, "rewrite the documentation figures in docs/img")

// The figures in docs/screens.md are drawn by the TUI itself, so a page cannot
// show a screen the code stopped producing. Without -figures this only reports
// that one is stale; with it, they are rewritten.
func TestDocumentationFigures(t *testing.T) {
	profile, dark := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(profile)
		lipgloss.SetHasDarkBackground(dark)
	})
	for _, f := range figures() {
		t.Run(f.name, func(t *testing.T) {
			screen := f.build(t).View()
			if testing.Verbose() {
				for i, line := range strings.Split(screen, "\n") {
					t.Logf("%2d|%s|", i, ansi.Strip(line))
				}
			}
			svg := ansisvg.Render(screen, ansisvg.Options{
				Title:  f.title,
				Labels: resolveNotes(t, screen, f.notes),
			})
			path := filepath.Join("..", "..", "docs", "img", f.name+".svg")
			if *writeFigures {
				if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err, "\nrun: go test ./internal/tui -run TestDocumentationFigures -figures")
			}
			if string(want) != svg {
				t.Fatal(f.name, "is stale; rerun: go test ./internal/tui -run TestDocumentationFigures -figures")
			}
		})
	}
}

type figure struct {
	name, title string
	notes       []note
	build       func(*testing.T) *model
}

// A note names a component beside the row it occupies. The row is found by
// what it says rather than counted, so a figure that gains a line does not
// quietly move every label one component down.
type note struct{ anchor, text string }

func resolveNotes(t *testing.T, screen string, notes []note) []ansisvg.Label {
	t.Helper()
	rows := strings.Split(screen, "\n")
	var out []ansisvg.Label
	for _, n := range notes {
		row := -1
		for i, line := range rows {
			if strings.Contains(ansi.Strip(line), n.anchor) {
				row = i
				break
			}
		}
		if row < 0 {
			t.Fatalf("no row says %q, so the label %q has nothing to point at", n.anchor, n.text)
		}
		out = append(out, ansisvg.Label{Row: row, Text: n.text})
	}
	return out
}

func figures() []figure {
	return []figure{{
		name:  "session",
		title: "A cxz session: the project panel, a conversation, the composer and the status bar",
		notes: []note{
			{"Projects", "project panel · cxz project ls"},
			{"notes · codex", "a session · cxz session ls"},
			{"Why does the composer", "your message · cxz session send"},
			{"Read internal", "tool activity · /details"},
			{"CLAUDE", "agent reply"},
			{"composerRows", "code block · Ctrl+C copies"},
			{"¢", "turn metrics · /usage"},
			{"run the composer tests", "composer · Ctrl+S sends"},
			{"claude-opus-4-6", "model · /model"},
		},
		build: sessionFigure,
	}, {
		name:  "approval",
		title: "A cxz session holding a pending approval",
		notes: []note{
			{"Bash", "the request · cxz session get"},
			{"Tab focus", "allow or deny · cxz session reply"},
		},
		build: approvalFigure,
	}, {
		name:  "projects",
		title: "The cxz project list",
		notes: []note{
			{"roster", "a project · cxz project ls"},
			{"layout · claude", "a session · cxz session ls"},
			{"new · accounts", "keys for the list"},
		},
		build: projectsFigure,
	}}
}

// sessionFigure is a finished turn: a question, two tool calls, a reply with a
// code block, and the metrics the turn cost. Everything that would otherwise
// come from the clock -- timestamps, the spinner's phase, the quota countdown
// -- is fixed or derived from the moment the figure is drawn, so the same
// screen comes out of every run.
func sessionFigure(t *testing.T) *model {
	t.Helper()
	m := conversationModel()
	m.Update(tea.WindowSizeMsg{Width: 118, Height: 28})
	s := m.current()
	s.Title, s.Alias, s.Model, s.Account = "composer layout", "layout", "claude-opus-4-6", "personal"
	m.sessions = append(m.sessions, &api.Session{Id: "t", ProjectId: "p", RunId: "run2", Agent: "codex",
		State: "working", Title: "release notes", Alias: "notes", Account: "work", CreatedAt: 1772000000000})
	m.pulse = 0
	at := time.Date(2026, 3, 4, 14, 32, 0, 0, time.Local).UnixMilli()
	result, _ := json.Marshal(map[string]any{"type": "result", "duration_ms": 42300, "total_cost_usd": 0.0731,
		"usage":      map[string]any{"input_tokens": 18400, "output_tokens": 1240, "cache_read_input_tokens": 120000, "cache_creation_input_tokens": 3100},
		"modelUsage": map[string]any{"claude": map[string]any{"contextWindow": 200000}}})
	now := time.Now()
	quota := fmt.Sprintf(`{"rate_limits":{"five_hour":{"utilization":58,"resets_at":%q},"seven_day":{"utilization":12,"resets_at":%q}}}`,
		now.Add(3*time.Hour+57*time.Minute+30*time.Second).Format(time.RFC3339), now.Add(100*time.Hour).Format(time.RFC3339))
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "run", Kind: "models", Payload: []byte(`{"Model":"claude-opus-4-6","Effort":"high"}`), TimeMs: at},
		{Seq: 2, RunId: "run", Kind: "input", Text: "Why does the composer stutter when a line wraps?", TimeMs: at},
		{Seq: 3, RunId: "run", Kind: "tool_call", Text: "Read", RequestId: "r1", Payload: []byte(`{"file_path":"internal/tui/style.go"}`), TimeMs: at + 2000},
		{Seq: 4, RunId: "run", Kind: "tool_result", RequestId: "r1", Text: "ok", TimeMs: at + 3000},
		{Seq: 5, RunId: "run", Kind: "tool_call", Text: "Edit", RequestId: "r2", Payload: []byte(`{"file_path":"internal/tui/style.go","old_string":"abc","new_string":"abcd"}`), TimeMs: at + 4000},
		{Seq: 6, RunId: "run", Kind: "tool_result", RequestId: "r2", Text: "ok", TimeMs: at + 5000},
		{Seq: 7, RunId: "run", Kind: "assistant", TimeMs: at + 6000,
			Text: "The height was **estimated** rather than measured:\n\n- a row that fills exactly takes two rows\n- words wrap before the edge\n\n```go\nrows := len(m.composerRows())\n```"},
		{Seq: 8, RunId: "run", Kind: "turn_end", Payload: result, TimeMs: at + 7000},
		{Seq: 9, RunId: "run", Kind: "usage", Text: "get_usage", TimeMs: now.UnixMilli(), Payload: []byte(quota)},
	}
	m.cursor["s"] = 9
	m.render()
	m.input.SetValue("run the composer tests")
	m.resize()
	return m
}

func approvalFigure(t *testing.T) *model {
	t.Helper()
	m := sessionFigure(t)
	m.Update(tea.WindowSizeMsg{Width: 118, Height: 34})
	s := m.current()
	s.PermissionMode = "ask"
	request := &api.Event{Seq: 10, RunId: "run", Kind: "approval", Text: "Bash", RequestId: "a1",
		Payload: []byte(`{"command":"go test ./internal/tui","description":"run the composer tests"}`)}
	s.Pending = []*api.Event{request}
	m.events["s"] = append(m.events["s"], request)
	m.cursor["s"] = 10
	m.input.Reset()
	m.render()
	m.resize()
	return m
}

// The list is its own screen when the terminal is too narrow to hold it beside
// a conversation, which is the shape worth drawing: the same rows, alone.
func projectsFigure(t *testing.T) *model {
	t.Helper()
	m := sessionFigure(t)
	m.panelProjects = append(m.panelProjects, &api.Project{Id: "q", Name: "roster", Alias: "roster", Workspace: "/work/roster", State: "running"})
	m.allSessions = append(append([]*api.Session(nil), m.sessions...),
		&api.Session{Id: "u", ProjectId: "q", RunId: "run3", Agent: "claude", State: "idle", Title: "schema review", Alias: "schema", CreatedAt: 1772000100000},
		&api.Session{Id: "v", ProjectId: "q", RunId: "run4", Agent: "codex", State: "stopped", Title: "flaky test", Alias: "flaky", CreatedAt: 1772000200000})
	m.Update(tea.WindowSizeMsg{Width: 64, Height: 20})
	m.panelFocus = true
	m.render()
	return m
}
