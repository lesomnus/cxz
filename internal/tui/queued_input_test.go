package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// A message cxz is holding has not entered the conversation, so it is not drawn
// into it. It is said above the composer instead, where it can be taken back.
func TestWaitingMessageIsSaidAndTakenBack(t *testing.T) {
	m := conversationModel()
	c := m.client.(*recordingClient)
	c.status = "queued"
	m.input.SetValue("also fix the test")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("the composer did not send")
	}
	m.Update(cmd())
	if len(m.pendingInputs["s"]) != 0 {
		t.Fatal("a waiting message was echoed into the conversation")
	}
	// The session carries it, so a second frontend shows the same thing.
	m.current().Queued = "also fix the test"
	status, message, _ := m.composerStatus()
	if text := ansi.Strip(status); !strings.Contains(text, "waiting to send") || !strings.Contains(text, "also fix the test") {
		t.Fatal("the status row does not say what is waiting:", text)
	}
	if message != "also fix the test" {
		t.Fatal("the row cannot be clicked for the full text:", message)
	}
	// Ctrl+X clears a draft; with nothing to clear it takes back the other one.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if cmd == nil {
		t.Fatal("Ctrl+X did not take the waiting message back")
	}
	msg := cmd()
	last := c.inputs[len(c.inputs)-1]
	if !last.Cancel || last.Text != "" {
		t.Fatal("cancelling sent a message instead:", last)
	}
	m.Update(msg)
	if m.input.Value() != "also fix the test" {
		t.Fatal("the cancelled message did not come back to the composer:", m.input.Value())
	}
}

// A draft the server refuses is the user's text, not ours to drop.
func TestRefusedMessageReturnsToTheComposer(t *testing.T) {
	m := conversationModel()
	c := m.client.(*recordingClient)
	c.sendErr = errors.New("a message is already waiting for the agent; cancel it or wait")
	m.input.SetValue("and another")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("the composer did not send")
	}
	if m.input.Value() != "" {
		t.Fatal("the composer kept the text before the server answered")
	}
	m.Update(cmd())
	if m.input.Value() != "and another" {
		t.Fatal("the refused text was lost:", m.input.Value())
	}
	if len(m.pendingInputs["s"]) != 0 {
		t.Fatal("the refused message is still echoed in the conversation")
	}
	// Typing while the answer was in flight wins: nothing overwrites it.
	m.input.SetValue("something newer")
	m.Update(result{inputSession: "s", inputRequest: "r", inputText: "older", err: errors.New("refused")})
	if m.input.Value() != "something newer" {
		t.Fatal("a late refusal overwrote the composer:", m.input.Value())
	}
}

// Ctrl+X keeps clearing the draft; taking a message back is what it does only
// when there is no draft to clear.
func TestCancelDoesNotStealTheClearKey(t *testing.T) {
	m := conversationModel()
	c := m.client.(*recordingClient)
	m.current().Queued = "waiting"
	m.input.SetValue("still typing")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if cmd != nil {
		t.Fatal("Ctrl+X cancelled while there was a draft to clear")
	}
	if m.input.Value() != "" {
		t.Fatal("Ctrl+X did not clear the draft:", m.input.Value())
	}
	if len(c.inputs) != 0 {
		t.Fatal("clearing a draft reached the server")
	}
}
