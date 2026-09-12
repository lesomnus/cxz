package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func TestEnterNewlineAndSend(t *testing.T) {
	m := conversationModel()
	c := m.client.(*recordingClient)
	m.input.SetValue("hello")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.input.Value() != "hello\n" || len(c.inputs) != 0 {
		t.Fatal("Enter submitted instead of newline")
	}
	m.input.InsertString("world")
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	cmd()
	if len(c.inputs) != 1 || c.inputs[0].Text != "hello\nworld" {
		t.Fatal(c.inputs)
	}
	m.input.SetValue("/he")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.input.Value() != "/he\n" {
		t.Fatal("hint changed Enter newline semantics")
	}
}

func TestFocusPhysicalOrderAndReverse(t *testing.T) {
	for _, pending := range []bool{false, true} {
		m := conversationModel()
		if pending {
			m.current().Pending = []*api.Event{{RequestId: "r", Text: "Bash"}}
		}
		order := []string{"session", "input"}
		if pending {
			order = []string{"session", "approval", "input"}
		}
		focus := func() string {
			if m.focusList {
				return "session"
			}
			if m.focusApproval {
				return "approval"
			}
			return "input"
		}
		for _, want := range order {
			m.Update(tea.KeyMsg{Type: tea.KeyTab})
			if focus() != want {
				t.Fatalf("want %s got %s", want, focus())
			}
		}
		for i := len(order) - 2; i >= -1; i-- {
			want := "input"
			if i >= 0 {
				want = order[i]
			}
			m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
			if focus() != want {
				t.Fatalf("reverse want %s got %s", want, focus())
			}
		}
	}
}

func TestInterruptConfirmationExpiryAndIdentity(t *testing.T) {
	m := conversationModel()
	m.current().State = "working"
	c := m.client.(*recordingClient)
	now := time.Now()
	if m.confirmInterrupt(now) != nil {
		t.Fatal("first Esc interrupted")
	}
	if !strings.Contains(m.workingLabel(now), "Esc again") {
		t.Fatal("no confirmation hint")
	}
	if strings.Contains(m.workingLabel(now.Add(3*time.Second)), "Esc again") {
		t.Fatal("confirmation did not expire")
	}
	if m.confirmInterrupt(now.Add(3*time.Second)) != nil {
		t.Fatal("expired Esc interrupted")
	}
	cmd := m.confirmInterrupt(now.Add(4 * time.Second))
	if cmd == nil {
		t.Fatal("second Esc ignored")
	}
	cmd()
	if len(c.interrupts) != 1 {
		t.Fatal("interrupt not dispatched")
	}
	m.confirmInterrupt(now)
	m.current().RunId = "replacement"
	if m.confirmInterrupt(now.Add(time.Second)) != nil {
		t.Fatal("confirmed a different run")
	}
	m.Update(received{id: "s", event: &api.Event{Seq: 1, Kind: "turn_end"}})
	if !m.interruptUntil.IsZero() {
		t.Fatal("confirmation leaked to next turn")
	}
}

func TestFuzzyHintsViewportAndMargin(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("/ctxt")
	hints := m.commandHints()
	if len(hints) != 1 || hints[0].name != "/context" {
		t.Fatal(hints)
	}
	m.input.SetValue("/")
	rows := strings.Split(m.commandOverlay(strings.Repeat("transcript\n", 11)+"transcript"), "\n")
	if strings.TrimSpace(rows[4]) != "" {
		t.Fatal("missing gap above seven hints", rows)
	}
	if strings.Count(strings.Join(rows, "\n"), "/") != 7 {
		t.Fatal("more than seven hints visible", rows)
	}
	for _, selected := range []int{4, 5, 6, 7, 8, 7, 6, 5, 4, 3, 2, 1, 0} {
		m.hintSelected = selected
		m.commandOverlay(strings.Repeat("transcript\n", 11) + "transcript")
		if selected >= 2 && selected <= len(slashCommands)-3 && (selected-m.hintOffset < 2 || m.hintOffset+6-selected < 2) {
			t.Fatalf("no two-row margin: selected %d offset %d", selected, m.hintOffset)
		}
	}
	if fuzzyScore("/context", "/context") >= fuzzyScore("/context", "/cont") || fuzzyScore("/context", "/zz") >= 0 {
		t.Fatal("fuzzy ordering")
	}
}

func TestScrollableApprovalAndResolvedRow(t *testing.T) {
	m := conversationModel()
	request := &api.Event{Seq: 1, RunId: "run", Kind: "approval", RequestId: "r", Text: "Bash", Payload: []byte(`{"input":{"command":"echo start"},"zdata":"` + strings.Repeat("한글 ", 150) + `TAIL"}`)}
	m.current().Pending = []*api.Event{request}
	m.events["s"] = []*api.Event{request}
	m.resize()
	m.render()
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if !strings.Contains(ansi.Strip(m.approvalBox()), "│  Pending") {
		t.Fatal("missing gutter")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlEnd})
	box := ansi.Strip(m.approvalBox())
	if !strings.Contains(box, "TAIL") || m.approvalOffset == 0 {
		t.Fatal("payload tail inaccessible", box)
	}
	m.Update(received{id: "s", event: &api.Event{Seq: 2, RunId: "run", Kind: "approval_resolved", RequestId: "r", Text: "allowed"}})
	m.view.GotoTop()
	text := ansi.Strip(m.view.View())
	if !strings.Contains(text, "🗹 approval allowed") || strings.Contains(text, "approval requested") || strings.Contains(text, "approval: allowed") {
		t.Fatal(text)
	}
	if request.Kind != "approval" || !strings.Contains(string(request.Payload), "TAIL") {
		t.Fatal("journal event mutated")
	}
}

func TestToolResultCompactAndDetails(t *testing.T) {
	m := conversationModel()
	e := &api.Event{Seq: 1, Kind: "tool_result", Payload: []byte(`"first\n` + strings.Repeat(`line\n`, 100) + `TAIL"`)}
	m.events["s"] = []*api.Event{e}
	text := ansi.Strip(eventView(m.current(), e, 80))
	if strings.Contains(text, "TAIL") || strings.Contains(text, "\n") || !strings.Contains(text, "/details") {
		t.Fatal(text)
	}
	m.toolDetails()
	if !strings.Contains(m.localReports["s"], "TAIL") {
		t.Fatal("details lost")
	}
}

func TestContextAndCompactCommands(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		m := conversationModel()
		m.current().Agent = agent
		c := m.client.(*recordingClient)
		m.events["s"] = []*api.Event{{Kind: "usage", Text: "thread/tokenUsage/updated", Payload: []byte(`{"tokenUsage":{"last":{"totalTokens":2000,"inputTokens":1800},"modelContextWindow":10000}}`)}}
		cmd := m.contextCommand()
		if agent == "claude" {
			if cmd == nil {
				t.Fatal("native context missing")
			}
			cmd()
			if c.inputs[0].Text != "/context" {
				t.Fatal(c.inputs)
			}
		} else if cmd != nil || !strings.Contains(m.localReports["s"], "20.0%") {
			t.Fatal(m.localReports)
		}
		cmd = m.compactCommand("/compact")
		if cmd == nil {
			t.Fatal("no compaction")
		}
		cmd()
		if c.inputs[len(c.inputs)-1].Text != "/compact" {
			t.Fatal(c.inputs)
		}
		m.current().State = "working"
		if m.compactCommand("/compact") != nil {
			t.Fatal("compacted active turn")
		}
	}
}

func TestQuotaFooterAndStaleness(t *testing.T) {
	m := conversationModel()
	m.current().Agent = "codex"
	now := time.Unix(1900000000, 0)
	m.events["s"] = []*api.Event{{Kind: "usage", Text: "account/rateLimits/updated", TimeMs: now.UnixMilli(), Payload: []byte(`{"rateLimits":{"primary":{"usedPercent":60,"windowDurationMins":300,"resetsAt":1900010320},"secondary":{"usedPercent":54,"windowDurationMins":10080,"resetsAt":1900536400}}}`)}}
	m.updateQuota()
	text := ansi.Strip(m.quotaStatus(now, 100))
	if !strings.Contains(text, "40%") || !strings.Contains(text, "46%") || !strings.Contains(text, "5h 2h52m") || !strings.Contains(text, "wk") {
		t.Fatal(text)
	}
	if !strings.Contains(m.quotaStatus(now.Add(3*time.Minute), 100), "~40%") {
		t.Fatal("stale quota not marked")
	}
	if strings.Contains(m.quotaStatus(now.Add(8*24*time.Hour), 100), "40%") {
		t.Fatal("expired quota presented as current")
	}
	for _, width := range []int{40, 80, 140} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		rows := strings.Split(m.View(), "\n")
		if ansi.StringWidth(rows[len(rows)-1]) != width || !strings.HasSuffix(ansi.Strip(rows[len(rows)-1]), " ") || strings.HasSuffix(ansi.Strip(rows[len(rows)-1]), "  ") {
			t.Fatal("footer not right aligned")
		}
	}
}

func TestQuotaAvailabilityDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		state  string
		events []*api.Event
		hint   string
	}{
		{"waiting", nil, "Existing supervisors keep their old code"},
		{"unsupported", []*api.Event{{Kind: "usage_status", Text: "unsupported", RunId: "run"}}, "Update the provider CLI"},
		{"unavailable", []*api.Event{{Kind: "usage", Text: "get_usage", Payload: []byte(`{"rate_limits_available":false,"rate_limits":null}`)}}, "no quota windows"},
		{"error", []*api.Event{{Kind: "usage_status", Text: "error", RunId: "run"}}, "will retry"},
		{"waiting", []*api.Event{{Kind: "usage_status", Text: "unsupported", RunId: "old"}}, "Existing supervisors keep their old code"},
	} {
		m := conversationModel()
		m.events["s"] = tc.events
		m.updateQuota()
		if m.quotaState != tc.state || !strings.Contains(ansi.Strip(m.quotaStatus(time.Now(), 40)), tc.state) {
			t.Fatal("wrong quota state", m.quotaState)
		}
		if report := quotaHistoryReport("claude", "run", tc.events); !strings.Contains(report, tc.hint) {
			t.Fatal(report)
		}
	}
	events := []*api.Event{
		{Kind: "usage_status", Text: "error"},
		{Kind: "usage", Text: "get_usage", Payload: []byte(`{"rate_limits":{"five_hour":{"utilization":30}}}`)},
	}
	windows, state := quotaSnapshot("claude", "run", events)
	if state != "available" || len(windows) != 1 || *windows[0].Remaining != 70 {
		t.Fatal("quota did not recover")
	}
}
