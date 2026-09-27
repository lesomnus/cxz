package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

func TestContextSummaryAndRawInteractions(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		m := conversationModel()
		m.current().Agent = provider
		m.input.SetValue("keep draft")
		m.view.SetContent(strings.Repeat("existing history\n", 100))
		m.view.SetYOffset(12)
		raw := "## Context Usage\n**Model:** test\n**Tokens:** 2k / 10k (20%)\n"
		r := agentview.ParseClaudeContext(raw)
		if provider == "codex" {
			raw = `{"tokenUsage":{"last":{"totalTokens":2000},"modelContextWindow":10000}}`
			r = agentview.ParseCodexContext([]byte(raw))
		}
		m.openReport("/context", "")
		m.report.setContext(r, raw, 0)
		if !strings.Contains(m.report.text, "Utilization 20.0%") || !strings.Contains(m.report.text, "Available   8k tokens") {
			t.Fatal(m.report.text)
		}
		for _, width := range []int{20, 80, 120} {
			m.width = width
			view := ansi.Strip(m.reportView(m.view.View()))
			if !strings.Contains(view, "[Summary] [Raw]") {
				t.Fatal(view)
			}
			for _, row := range strings.Split(view, "\n") {
				if strings.ContainsAny(row, "╭╰│") && ansi.StringWidth(row) > width {
					t.Fatal("overflow", row)
				}
			}
			y := m.report.context.headerY
			if !m.contextReportMouse(tea.MouseMsg{X: 13, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}) || !m.report.context.rawMode {
				t.Fatal("Raw button not clickable", width, y)
			}
			view = ansi.Strip(m.reportView(m.view.View()))
			if !strings.Contains(view, "tokenUsage") && !strings.Contains(view, "Context Usage") {
				t.Fatal("raw lost", view)
			}
			m.reportKey(tea.KeyMsg{Type: tea.KeyLeft})
			if m.report.context.rawMode {
				t.Fatal("left did not return to Summary")
			}
		}
		m.report.offset = 3
		m.reportKey(tea.KeyMsg{Type: tea.KeyRight})
		m.report.offset = 8
		m.reportKey(tea.KeyMsg{Type: tea.KeyTab})
		if m.report.context.rawMode || m.report.offset != 3 {
			t.Fatal("summary scroll not preserved")
		}
		m.reportKey(tea.KeyMsg{Type: tea.KeyRight})
		if m.report.offset != 8 {
			t.Fatal("raw scroll not preserved")
		}
		m.reportKey(tea.KeyMsg{Type: tea.KeyEsc})
		if m.report != nil || m.view.YOffset != 12 || m.input.Value() != "keep draft" {
			t.Fatal("modal changed transcript/draft")
		}
	}
}
func TestContextIgnoresOtherRunAndPreCompactSnapshots(t *testing.T) {
	m := conversationModel()
	m.current().Agent = "codex"
	payload := []byte(`{"tokenUsage":{"last":{"totalTokens":2000},"modelContextWindow":10000}}`)
	m.events["s"] = []*api.Event{{RunId: "old", Kind: "usage", Text: "thread/tokenUsage/updated", Payload: payload}}
	m.contextCommand()
	if m.report.context.snapshot.Used != nil {
		t.Fatal("old run usage displayed")
	}
	m.events["s"] = []*api.Event{{RunId: m.current().RunId, Kind: "usage", Text: "thread/tokenUsage/updated", Payload: payload}, {RunId: m.current().RunId, Kind: "compact"}}
	m.contextCommand()
	if m.report.context.snapshot.Used != nil || !strings.Contains(m.report.text, "compacted") {
		t.Fatal("pre-compact usage displayed")
	}
}
func TestUnrecognizedContextKeepsRawAndClosedModalClosed(t *testing.T) {
	m := conversationModel()
	m.current().Agent = "claude"
	cmd := m.contextCommand()
	cmd()
	request := m.client.(*recordingClient).inputs[0].ClientId
	report := m.report
	m.reportKey(tea.KeyMsg{Type: tea.KeyEsc})
	for _, e := range []*api.Event{
		{RunId: m.current().RunId, Kind: "input", RequestId: request},
		{RunId: m.current().RunId, Kind: "assistant", Text: "new native format"},
		{RunId: m.current().RunId, Kind: "turn_end"},
	} {
		m.captureContext("s", e)
	}
	if m.report != nil || report.context.raw != "new native format" || report.context.snapshot.Used != nil || !strings.Contains(report.text, "not recognized") {
		t.Fatal("fallback or closed modal broken", report)
	}
}

func TestContextRawClickThroughUpdate(t *testing.T) {
	m := conversationModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 36})
	m.openReport("/context", "")
	m.report.setContext(agentview.ContextReport{Provider: "codex", Basis: "Last reported turn"}, "original payload", 0)
	m.View()
	y := m.report.context.headerY
	m.Update(tea.MouseMsg{X: m.contentOffset() + 13, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.report.context.rawMode {
		t.Fatal("Raw click was not routed to the report", y)
	}
	m.View()
	m.Update(tea.MouseMsg{X: m.contentOffset() + 4, Y: m.report.context.headerY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.report.context.rawMode {
		t.Fatal("Summary click was not routed to the report")
	}
}
