package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

func TestMCPApprovalLabelAndDoubleClick(t *testing.T) {
	for _, state := range []string{"allowed", "denied", "canceled"} {
		m := conversationModel()
		m.current().Agent = "codex"
		raw := []byte(`{"params":{"serverName":"cxz_memory","mode":"form","message":"Allow memory_update?\nSave project decisions.","requestedSchema":{"type":"object","properties":{}},"future":"retained"}}`)
		m.events["s"] = []*api.Event{
			{Seq: 1, RunId: "run", RequestId: "mcp-1", Kind: "approval", Text: agentview.CodexElicitation, Payload: raw},
			{Seq: 2, RunId: "run", RequestId: "mcp-1", Kind: "approval_resolved", Text: state},
			{Seq: 3, RunId: "other-run", RequestId: "mcp-1", Kind: "approval_resolved", Text: "wrong-run"},
		}
		m.render()
		m.view.GotoTop()
		view := ansi.Strip(m.view.View())
		if !strings.Contains(view, "MCP · cxz_memory") || !strings.Contains(view, "Allow memory_update?") {
			t.Fatalf("missing MCP identity/message: %q", view)
		}
		row := -1
		for y, seq := range m.toolRows {
			if seq == 1 {
				row = y
				break
			}
		}
		if row < 0 {
			t.Fatal("approval has no inspection hitbox")
		}
		click := tea.MouseMsg{X: m.contentOffset() + 4, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
		m.Update(click)
		if m.filePreview != nil {
			t.Fatal("single click opened approval")
		}
		m.Update(tea.MouseMsg{X: click.X, Y: click.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
		m.Update(click)
		p := m.filePreview
		if p == nil || !strings.Contains(p.title, "cxz_memory") || !strings.Contains(p.source, "Approval status: "+state) || !strings.Contains(p.source, `"future": "retained"`) || !strings.Contains(p.source, "Save project decisions.") {
			t.Fatalf("incomplete approval preview: %+v", p)
		}
		if strings.Contains(p.source, "wrong-run") {
			t.Fatal("matched another run's decision")
		}
	}
}

func TestMCPApprovalMissingNameAndResolution(t *testing.T) {
	m := conversationModel()
	m.current().Agent = "codex"
	e := &api.Event{Seq: 1, Kind: "approval", Text: agentview.CodexElicitation, Payload: []byte(`{"params":{"mode":"form"}}`)}
	m.events["s"] = []*api.Event{e}
	if got := ansi.Strip(approvalLine(m.current(), e, "allowed", 80)); strings.Contains(got, "·") || !strings.Contains(got, "✓ MCP") {
		t.Fatal(got)
	}
	p := m.previewForSequence(1)
	if p == nil || !strings.Contains(p.source, "No resolution recorded in loaded history.") {
		t.Fatal(p)
	}
}
