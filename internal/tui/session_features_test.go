package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type featuresClient struct {
	recordingClient
	renamed      string
	renameErr    error
	pages        []*api.EventBatch
	historyCalls int
}

func (c *featuresClient) RenameSession(_ context.Context, id, alias string) (*api.Session, error) {
	c.renamed = id
	return &api.Session{Id: id, Alias: alias}, c.renameErr
}
func (c *featuresClient) History(context.Context, *api.WatchRequest, ...grpc.CallOption) (*api.EventBatch, error) {
	c.historyCalls++
	if len(c.pages) == 0 {
		return &api.EventBatch{}, nil
	}
	p := c.pages[0]
	c.pages = c.pages[1:]
	return p, nil
}
func TestRenameInProjectPanel(t *testing.T) {
	m := projectModel()
	c := &featuresClient{}
	m.client = c
	m.projectView = false
	m.input = newComposer()
	m.sessions = []*api.Session{{Id: "s", ProjectId: "p", Alias: "oak", Agent: "codex"}}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	panel := func() string { return ansi.Strip(m.panelScreen()) }
	if !strings.Contains(panel(), "oak · codex") {
		t.Fatal(panel())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if !m.renaming || !m.aliasInput.Focused() {
		t.Fatal("editor not focused")
	}
	for _, word := range []string{"oak", "orchard"} {
		m.aliasInput.SetValue(word)
		if !strings.Contains(panel(), word) {
			t.Fatal("panel editor lost alias", panel())
		}
	}
	for _, bad := range []string{"ab", "my_work", "2nd", "web-"} {
		m.aliasInput.SetValue(bad)
		if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil || !m.renaming {
			t.Fatal("invalid alias accepted", bad)
		}
	}
	// Digits and hyphens are what the resource layer already stores.
	m.aliasInput.SetValue("web-2")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(cmd())
	if m.renaming || m.current().Alias != "web-2" {
		t.Fatal("hyphenated alias rejected", m.notice)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m.aliasInput.SetValue("clover")
	c.renameErr = errors.New("already in use")
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(cmd())
	if !m.renaming || m.aliasInput.Value() != "clover" {
		t.Fatal("error discarded editor")
	}
	c.renameErr = nil
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(cmd())
	if m.renaming || !m.panelFocus || m.current().Alias != "clover" || c.renamed != "s" {
		t.Fatal("rename failed")
	}
	if !strings.Contains(panel(), "clover · codex") {
		t.Fatal(panel())
	}
}

func TestUsageFullJournal(t *testing.T) {
	c := &featuresClient{pages: []*api.EventBatch{
		{Events: []*api.Event{{Seq: 1, Kind: "input", TimeMs: 1000}, {Seq: 2, Kind: "usage", Text: "thread/tokenUsage/updated", Payload: []byte(`{"tokenUsage":{"last":{"inputTokens":1000,"outputTokens":20},"total":{"inputTokens":99999}}}`)}}},
		{Events: []*api.Event{{Seq: 3, Kind: "turn_end", Text: "completed", TimeMs: 3000}, {Seq: 4, Kind: "turn_end", Text: "completed", Payload: []byte(`{"usage":{"input_tokens":10},"total_cost_usd":0.5,"duration_ms":1000}`)}}},
	}}
	m := projectModel()
	m.client = c
	m.projectView = false
	m.input = newComposer()
	m.sessions = []*api.Session{{Id: "s"}}
	m.input.SetValue("/usage")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m.Update(cmd())
	text := m.usageReports["s"]
	for _, want := range []string{"2 finished turns", "1k", "$0.5000", "00:00:03", "1/2 turns reported"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	if strings.Contains(text, "99999") || c.historyCalls != 3 || len(c.inputs) > 0 || m.cursor["s"] != 0 {
		t.Fatal("bad usage pagination/side effect")
	}
}

func TestComposerNumbers(t *testing.T) {
	input := newComposer()
	input.SetValue("first\nsecond\nthird\n4\n5\n6\n7\n8\n9\n10\n11\n12")
	input.SetHeight(12)
	text := ansi.Strip(input.View())
	for _, want := range []string{"> first", "1 second", "2 third", "9 10", "0 11", "1 12"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
}

func TestUsageCumulativeClaudeCost(t *testing.T) {
	events := []*api.Event{
		{Kind: "turn_end", RunId: "one", Payload: []byte(`{"total_cost_usd":0.1}`)},
		{Kind: "turn_end", RunId: "one", Payload: []byte(`{"total_cost_usd":0.2}`)},
		{Kind: "turn_end", RunId: "two", Payload: []byte(`{"total_cost_usd":0.05}`)},
	}
	if got := usageReport(events); !strings.Contains(got, "$0.2500") {
		t.Fatal("cumulative cost counted twice", got)
	}
}
