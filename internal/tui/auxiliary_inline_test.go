package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"google.golang.org/grpc"
)

type inlineAIClient struct {
	api.SessionsClient
	requests []auxiliary.Request
	reply    auxiliary.Reply
}

func (c *inlineAIClient) Docker(_ context.Context, in *api.DockerInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	var r auxiliary.Request
	if err := json.Unmarshal(in.Spec, &r); err != nil {
		return nil, err
	}
	c.requests = append(c.requests, r)
	b, err := json.Marshal(c.reply)
	return &api.Receipt{Status: string(b)}, err
}
func inlineAIModel() (*model, *inlineAIClient) {
	m := conversationModel()
	c := &inlineAIClient{}
	m.client = c
	m.events["s"] = []*api.Event{{SessionId: "s", RunId: "run", Seq: 1, Kind: "input", Text: "question"}, {SessionId: "s", RunId: "run", Seq: 2, Kind: "assistant", Text: "final answer"}, {SessionId: "s", RunId: "run", Seq: 3, Kind: "turn_end", Text: "completed"}}
	m.render()
	return m, c
}
func TestAuxiliaryCommandsShowInlineLoadingWithoutSending(t *testing.T) {
	for _, command := range []string{"/summary", "/suggest"} {
		t.Run(command, func(t *testing.T) {
			m, c := inlineAIModel()
			m.input.SetValue(command)
			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
			if cmd == nil || m.report != nil || m.input.Value() != "" {
				t.Fatal("command did not start inline", m.report, m.input.Value())
			}
			if command == "/summary" {
				if len(m.auxiliaryLoadingRows) != 1 || !strings.Contains(ansi.Strip(m.conversationView()), "Summary") {
					t.Fatal("no inline loading")
				}
				view := ansi.Strip(m.view.View())
				if strings.Index(view, "Summary") < strings.Index(view, "final answer") {
					t.Fatal("summary preceded response", view)
				}
			} else if !strings.HasPrefix(m.suggestionGhost(), "Suggestion ") {
				t.Fatal("no ghost loading", m.suggestionGhost())
			}
			m.receiveAuxiliary(cmd().(auxiliaryResult))
			if len(c.requests) != 1 || c.requests[0].Action != "session" || c.requests[0].Enabled != nil {
				t.Fatal(c.requests)
			}
		})
	}
}
func TestAuxiliarySessionCommandArgumentsAndNoop(t *testing.T) {
	for _, command := range []string{"/summary on", "/summary off", "/suggest on", "/suggest off"} {
		m, c := inlineAIModel()
		cmd := m.auxiliaryCommand(command)
		if cmd == nil {
			t.Fatal(command)
		}
		m.receiveAuxiliary(cmd().(auxiliaryResult))
		r := c.requests[0]
		if r.Enabled == nil || *r.Enabled != strings.HasSuffix(command, " on") || r.Session != "s" {
			t.Fatal(r)
		}
	}
	m, c := inlineAIModel()
	key := m.connectionRef() + "/s"
	m.auxiliaryConfigs = map[string]auxiliary.SessionConfig{key: {Summary: true, Suggestion: true}}
	for _, command := range []string{"/summary", "/suggest"} {
		cmd := m.auxiliaryCommand(command)
		if m.auxiliaryJob() != nil || m.report != nil {
			t.Fatal("enabled command changed presentation")
		}
		m.receiveAuxiliary(cmd().(auxiliaryResult)) // only checks server state; no local loading/copy
	}
	if len(c.requests) != 2 {
		t.Fatal(c.requests)
	}
	if cmd := m.auxiliaryCommand("/summary invalid"); cmd != nil {
		t.Fatal("invalid args accepted")
	}
}
func TestAuxiliaryGhostHidesOnTypingAndIgnoresOldPoll(t *testing.T) {
	m, _ := inlineAIModel()
	key := m.connectionRef() + "/s"
	cmd := m.auxiliaryCommand("/suggest")
	if cmd == nil {
		t.Fatal("missing request")
	}
	m.receiveAuxiliary(auxiliaryResult{connection: m.connectionRef(), session: "s", action: "status"})
	if !auxiliaryLoading(m.auxiliaryJob()) {
		t.Fatal("older poll erased optimistic loading")
	}
	m.input.SetValue("한")
	if m.suggestionGhost() != "" {
		t.Fatal("loading overlaps draft")
	}
	m.input.Reset()
	m.receiveAuxiliary(auxiliaryResult{version: 1, connection: m.connectionRef(), session: "s", action: "session", reply: auxiliary.Reply{Job: &auxiliary.Job{ID: "done", Run: "run", Turn: 3, Status: "completed", Suggestion: "다음 작업을 진행해줘"}}})
	if m.suggestionGhost() != "다음 작업을 진행해줘" {
		t.Fatal(m.suggestionGhost())
	}
	m.input.SetValue("x")
	if m.suggestionGhost() != "" {
		t.Fatal("ghost overlaps draft")
	}
	m.input.Reset()
	m.events["s"] = append(m.events["s"], &api.Event{Kind: "input", Seq: 4})
	if m.suggestionGhost() != "" {
		t.Fatal("stale suggestion")
	}
	if m.auxiliaryPending[key] {
		t.Fatal("pending request stuck")
	}
}
func TestAuxiliarySummaryAnimationDoesNotRebuildOrScroll(t *testing.T) {
	m, _ := inlineAIModel()
	m.events["s"][1].Text = strings.Repeat("answer line\n", 100)
	m.render()
	m.view.SetYOffset(10)
	offset := m.view.YOffset
	m.auxiliaryCommand("/summary")
	if m.view.YOffset != offset {
		t.Fatal("loading changed scroll", offset, m.view.YOffset)
	}
	total := m.view.TotalLineCount()
	m.pulse = 8
	_ = m.conversationView()
	if m.view.TotalLineCount() != total || m.view.YOffset != offset {
		t.Fatal("animation changed geometry")
	}
	m.view.GotoBottom()
	first := m.conversationView()
	m.pulse = 12
	second := m.conversationView()
	if first == second {
		t.Fatal("loading not animated")
	}
	m.receiveAuxiliary(auxiliaryResult{version: 1, connection: m.connectionRef(), session: "s", action: "session", reply: auxiliary.Reply{Job: &auxiliary.Job{ID: "j", Run: "run", Turn: 3, Status: "running", SummaryRequested: true, SuggestionRequested: true, Summary: "요약 완료"}}})
	if len(m.auxiliaryLoadingRows) != 0 || !strings.Contains(ansi.Strip(m.conversationView()), "요약 완료") || !strings.HasPrefix(m.suggestionGhost(), "Suggestion ") {
		t.Fatal("did not display summary while suggestion runs")
	}
}

func TestSummaryRegenerationShowsLoadingOverSavedSummary(t *testing.T) {
	m, _ := inlineAIModel()
	key := m.connectionRef() + "/s"
	m.auxiliarySummaries = map[string][]auxiliary.Summary{key: {{Run: "run", Turn: 3, Text: "previous summary"}}}
	if cmd := m.auxiliaryCommand("/summary"); cmd == nil {
		t.Fatal("no generation")
	}
	if len(m.auxiliaryLoadingRows) != 1 {
		t.Fatal("old summary hid new loading")
	}
}

func TestManualSuggestionLoadsAlongsideRunningSummary(t *testing.T) {
	m, _ := inlineAIModel()
	key := m.connectionRef() + "/s"
	m.auxiliaryJobs = map[string]*auxiliary.Job{key: {ID: "running", Run: "run", Turn: 3, Status: "running", SummaryRequested: true}}
	if cmd := m.auxiliaryCommand("/suggest"); cmd == nil {
		t.Fatal("could not queue suggestion")
	}
	if !strings.HasPrefix(m.suggestionGhost(), "Suggestion ") || len(m.auxiliaryLoadingRows) != 1 {
		t.Fatal("both pending tasks must be visible")
	}
}
