package tui

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"strings"
	"testing"
)

func TestMCPElicitationVisibleInFullModeAndDismissWithoutFields(t *testing.T) {
	m := questionModel()
	s := m.current()
	s.Agent = "codex"
	s.PermissionMode = "full"
	s.State = "waiting_input"
	s.Pending = []*api.Event{{RunId: s.RunId, RequestId: "mcp", Text: agentview.CodexElicitation, Payload: []byte(`{"params":{"serverName":"cxz_memory","mode":"form","message":"Save?","requestedSchema":{"type":"object","properties":{"note":{"type":"string"}},"required":["note"]}}}`)}}
	if len(m.visibleApprovals(s)) != 1 {
		t.Fatal("MCP input hidden by full mode")
	}
	m.openQuestion(s.Pending[0])
	d := m.questionDialog
	if d == nil || len(d.questions) != 2 {
		t.Fatal("missing MCP form")
	}
	if v := ansi.Strip(m.sessionScreen()); !strings.Contains(v, "cxz_memory") || !strings.Contains(v, "Save?") {
		t.Fatal(v)
	}
	d.selected[0][2] = true
	if cmd := m.questionNext(); cmd == nil || !d.sending || d.page != 0 {
		t.Fatal("cancel demanded form fields")
	}
}
func TestMCPElicitationOptionalFieldCanAdvance(t *testing.T) {
	m := questionModel()
	s := m.current()
	s.Agent = "codex"
	s.Pending = []*api.Event{{RunId: s.RunId, RequestId: "mcp", Text: agentview.CodexElicitation, Payload: []byte(`{"params":{"serverName":"test","mode":"form","message":"Input","requestedSchema":{"type":"object","properties":{"note":{"type":"string"}}}}}`)}}
	m.openQuestion(s.Pending[0])
	d := m.questionDialog
	d.selected[0][0] = true
	m.questionNext()
	if d.page != 1 {
		t.Fatal("accept did not advance")
	}
	if cmd := m.questionNext(); cmd == nil || !d.sending {
		t.Fatal("blank optional input rejected")
	}
}
