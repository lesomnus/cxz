package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func TestTurnSummary(t *testing.T) {
	e := &api.Event{Kind: "turn_end", Text: "completed", Payload: []byte(`{"usage":{"input_tokens":1234,"output_tokens":321,"cache_read_input_tokens":100},"total_cost_usd":0.0123,"duration_ms":2400,"result":"do not duplicate reply"}`)}
	text := ansi.Strip(turnSummary(e, nil, 0, 100))
	for _, want := range []string{"↑ 1.2k", "↓ 321", "↺ 100", "$0.0123 USD", "◷ 2.4s"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s: %s", want, text)
		}
	}
	if strings.Contains(text, "completed") || strings.Contains(text, "duplicate") || strings.Contains(text, "{") {
		t.Fatal(text)
	}
	for _, line := range strings.Split(text, "\n") {
		if ansi.StringWidth(line) > 100 || strings.HasPrefix(line, " ") {
			t.Fatal("not left aligned", line)
		}
	}
	text = ansi.Strip(turnSummary(e, nil, 0, 40))
	for _, line := range strings.Split(text, "\n") {
		if ansi.StringWidth(line) > 40 || strings.HasPrefix(line, " ") {
			t.Fatal("narrow summary not aligned", line)
		}
	}
}

func TestCodexLastUsageAndMissingMetrics(t *testing.T) {
	e := &api.Event{Kind: "turn_end", Text: "completed", TimeMs: 3500, Payload: []byte(`{"turn":{"status":"completed"}}`)}
	usage := &api.Event{Payload: []byte(`{"tokenUsage":{"total":{"inputTokens":99999},"last":{"inputTokens":50,"outputTokens":7,"cachedInputTokens":0,"totalTokens":57}}}`)}
	text := ansi.Strip(turnSummary(e, usage, 1000, 100))
	for _, want := range []string{"↑ 50", "↓ 7", "∑ 57", "◷≈ 2.5s"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	if strings.Contains(text, "99999") || strings.Contains(text, "USD") {
		t.Fatal("invented/cumulative metrics", text)
	}
	if got := turnSummary(e, nil, 0, 80); got != "" {
		t.Fatal("empty completed turn should be hidden", got)
	}
	e.Payload = []byte(`{"usage":{"input_tokens":null,"output_tokens":-1},"costUSD":"unknown"}`)
	if got := turnSummary(e, nil, 0, 80); got != "" {
		t.Fatal("invalid metrics displayed", got)
	}
	e.Text = "failed"
	e.Payload = []byte(`{"turn":{"error":{"message":"permission denied"}}}`)
	if got := ansi.Strip(turnSummary(e, nil, 0, 80)); !strings.Contains(got, "! failed · permission denied") {
		t.Fatal("error hidden", got)
	}
}

func TestConversationAlignmentAndSummaryPlacement(t *testing.T) {
	m := projectModel()
	m.input = newComposer()
	m.projectView = false
	m.sessions = []*api.Session{{Id: "s", Agent: "claude"}}
	m.events["s"] = []*api.Event{
		{Kind: "input", Text: "prompt", TimeMs: 1000},
		{Kind: "assistant", Text: "reply"},
		{Kind: "turn_end", Text: "completed", TimeMs: 2500, Payload: []byte(`{"costUSD":0.1}`)},
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	text := ansi.Strip(m.View())
	if !strings.Contains(text, "\n> prompt") {
		t.Fatal("prompt left margin remains", text)
	}
	transcript := ansi.Strip(m.view.View())
	rows := strings.Split(transcript, "\n")
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	transcript = strings.Join(rows, "\n")
	if !strings.Contains(transcript, "  reply\n\n$") {
		t.Fatal("summary must follow a blank line", transcript)
	}
	for _, agent := range []string{"claude", "codex"} {
		got := eventView(&api.Session{Agent: agent}, &api.Event{Kind: "assistant", Text: "reply"}, 80)
		if !strings.HasSuffix(got, "\n"+answer.Render("  reply")) {
			t.Fatal("answer must use neutral white")
		}
	}
}
