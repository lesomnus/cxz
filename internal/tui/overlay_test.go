package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func TestReportModalPreservesConversationAndDraft(t *testing.T) {
	m := conversationModel()
	m.view.SetContent(strings.Repeat("existing conversation\n", 100))
	m.view.SetYOffset(12)
	m.input.SetValue("unfinished draft")
	m.openReport("/help", "")
	before := m.view.View()
	for _, width := range []int{20, 80, 120} {
		m.width = width
		view := ansi.Strip(m.reportView(m.view.View()))
		if !strings.Contains(view, "╭") || !strings.Contains(view, "╰") {
			t.Fatal("modal has no border")
		}
		for _, row := range strings.Split(view, "\n") {
			if strings.ContainsAny(row, "╭╰│") && ansi.StringWidth(row) > width {
				t.Fatal("border overflow", row)
			}
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.report.offset == 0 || m.view.YOffset != 12 {
		t.Fatal("modal scroll changed transcript")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.report != nil || m.view.View() != before || m.input.Value() != "unfinished draft" {
		t.Fatal("closing modal changed conversation or draft")
	}
}

func TestClosedUsageModalDoesNotReopen(t *testing.T) {
	m := conversationModel()
	m.usageGeneration = map[string]uint64{"s": 1}
	m.usageReports = map[string]string{}
	m.openReport("/usage", "Loading")
	m.report.generation = 1
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(usageLoaded{id: "s", generation: 1, text: "Completed"})
	if m.report != nil {
		t.Fatal("async result reopened closed modal")
	}
}

func TestClaudeContextIsCorrelatedAndJournalRetained(t *testing.T) {
	m := conversationModel()
	m.current().Agent = "claude"
	cmd := m.contextCommand()
	cmd()
	request := m.client.(*recordingClient).inputs[0].ClientId
	events := []*api.Event{
		{Seq: 1, RunId: "run", Kind: "assistant", Text: "unrelated before echo"},
		{Seq: 2, RunId: "run", Kind: "input", Text: "/context", RequestId: request},
		{Seq: 3, RunId: "run", Kind: "assistant", Text: "context breakdown"},
		{Seq: 4, RunId: "run", Kind: "turn_end", Text: "completed"},
	}
	for _, e := range events {
		m.Update(received{id: "s", event: e})
	}
	if !strings.Contains(m.report.text, "context breakdown") || strings.Contains(m.report.text, "unrelated") {
		t.Fatal("wrong context capture")
	}
	if len(m.events["s"]) != 4 || m.hiddenEvents[events[0]] || !m.hiddenEvents[events[2]] {
		t.Fatal("journal lost or unrelated response hidden")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if strings.Contains(m.view.View(), "context breakdown") || !strings.Contains(m.view.View(), "unrelated") {
		t.Fatal("context leaked into transcript")
	}
}
