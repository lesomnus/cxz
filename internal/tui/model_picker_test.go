package tui

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type catalogClient struct {
	recordingClient
	events []*api.Event
}

func TestClaudeEffortPickerUsesDefaultAndResolvedModel(t *testing.T) {
	for _, selected := range []string{"", "default", "claude-opus-5[1m]"} {
		m := conversationModel()
		payload, _ := json.Marshal(map[string]any{
			"model": selected, "effective_model": "claude-opus-5[1m]", "effective_effort": "high",
			"models": []map[string]any{{"id": "default", "resolved_id": "claude-opus-5[1m]", "efforts": []string{"low", "medium", "high", "xhigh", "max"}}},
		})
		client := &catalogClient{events: []*api.Event{{Seq: 1, RunId: "run", Kind: "models", Payload: payload}}}
		m.client = client
		m.Update(m.modelCommand("/effort")())
		if p := m.modelPicker; !slices.Equal(p.options(), []string{"low", "medium", "high", "xhigh", "max", "default"}) || p.options()[p.selected] != "high" {
			t.Fatal("default model lost its reasoning choices or current level", selected, p.options(), p.selected)
		}
		view := ansi.Strip(m.modelPickerOverlay(strings.Repeat("transcript\n", 20)))
		for _, want := range []string{"/effort · reasoning level", "Current: high · model default", "Model default (reset)"} {
			if !strings.Contains(view, want) {
				t.Fatalf("missing %q: %s", want, view)
			}
		}
		_, send := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if send == nil {
			t.Fatal("reasoning level could not be applied")
		}
		send()
		if len(client.inputs) != 1 || client.inputs[0].Text != "/effort high" {
			t.Fatal("selected reset instead of the actual reasoning level", client.inputs)
		}
	}
}

func TestEffortPickerDoesNotOfferOnlyDefault(t *testing.T) {
	m := conversationModel()
	client := &catalogClient{events: []*api.Event{{Seq: 1, RunId: "run", Kind: "models", Payload: []byte(`{"model":"haiku","models":[{"id":"haiku"}]}`)}}}
	m.client = client
	m.Update(m.modelCommand("/effort")())
	if len(m.modelPicker.options()) != 0 {
		t.Fatal("no capabilities became a default-only selector")
	}
	if view := ansi.Strip(m.modelPickerOverlay(strings.Repeat("transcript\n", 15))); !strings.Contains(view, "No reasoning levels") {
		t.Fatal(view)
	}
	_, send := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if send != nil || len(client.inputs) != 0 {
		t.Fatal("unsupported effort sent a command")
	}
}

func TestReasoningDisplayDistinguishesAppliedAndPreferredEffort(t *testing.T) {
	for _, tc := range []struct{ payload, want string }{
		{`{"effort":"high","effective_effort":"low"}`, "low"},
		{`{"effort":"high","effective_effort":""}`, ""},
		{`{"effort":"high"}`, "high"}, // Older confirmed catalogs remain readable.
		{`{"models":[{"id":"codex","default":true,"default_effort":"medium"}]}`, "medium"},
	} {
		var catalog modelCatalog
		if err := json.Unmarshal([]byte(tc.payload), &catalog); err != nil {
			t.Fatal(err)
		}
		if got := catalog.currentEffort(); got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.payload, got, tc.want)
		}
	}
}

func (c *catalogClient) History(_ context.Context, r *api.WatchRequest, _ ...grpc.CallOption) (*api.EventBatch, error) {
	if r.AfterSeq > 0 {
		return &api.EventBatch{}, nil
	}
	return &api.EventBatch{Events: c.events}, nil
}

func TestModelPickerNeverChatsWithoutCapability(t *testing.T) {
	for _, command := range []string{"/model", "/effort", "/model chosen", "/effort high"} {
		m := conversationModel()
		c := &catalogClient{}
		m.client = c
		cmd := m.modelCommand(command)
		_, next := m.Update(cmd())
		if next != nil || len(c.inputs) != 0 || m.modelPicker == nil || !strings.Contains(m.modelPicker.message, "no model-control") {
			t.Fatal("unsupported runtime received command", command)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if len(c.inputs) != 0 {
			t.Fatal("empty selector submitted chat")
		}
	}
}

func TestModelPickerSelectAndStaleRun(t *testing.T) {
	m := conversationModel()
	c := &catalogClient{events: []*api.Event{{Seq: 1, RunId: "run", Kind: "models", Payload: []byte(`{"models":[{"id":"first"},{"id":"second"}]}`)}}}
	m.client = c
	_, next := m.Update(m.modelCommand("/model")())
	if next != nil || len(c.inputs) != 0 {
		t.Fatal("opening selector is not read-only")
	}
	view := ansi.Strip(m.modelPickerOverlay(strings.Repeat("transcript\n", 15)))
	if !strings.Contains(view, "/model · select") || !strings.Contains(view, "Search:") {
		t.Fatal(view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, send := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if send == nil {
		t.Fatal("selection did not submit")
	}
	send()
	if len(c.inputs) != 1 || c.inputs[0].Text != "/model second" {
		t.Fatal(c.inputs)
	}
	m.Update(m.modelCommand("/model")())
	m.current().RunId = "changed"
	_, send = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if send != nil || !strings.Contains(m.modelPicker.message, "run changed") {
		t.Fatal("stale selector applied")
	}
}

func TestLocalReportsHaveSeparator(t *testing.T) {
	m := conversationModel()
	for _, command := range []string{"/approval", "/details", "/permission"} {
		m.recordLocal(command, "example")
		text := ansi.Strip(m.localCommandView("s"))
		if !strings.HasPrefix(text, "  ─") {
			t.Fatal("local output not separated", command)
		}
	}
}
