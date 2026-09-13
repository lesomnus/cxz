package tui

import (
	"context"
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
	for _, command := range []string{"/help", "/usage", "/permission"} {
		m.recordLocal(command, "example")
		text := ansi.Strip(m.localCommandView("s"))
		if !strings.HasPrefix(text, "  ─") {
			t.Fatal("local output not separated", command)
		}
	}
}
