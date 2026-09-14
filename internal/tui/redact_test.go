package tui

import (
	"context"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"strings"
	"testing"
)

type fakeSecretFiles struct {
	bodies  []string
	deleted []string
}

func (f *fakeSecretFiles) PutSecret(_ context.Context, _ context.Context, _ *api.Project, _ string, b []byte) (string, error) {
	f.bodies = append(f.bodies, string(b))
	return "/cxz/secrets/fixture/value", nil
}
func (f *fakeSecretFiles) DeleteSecret(_ context.Context, _ context.Context, _ *api.Project, path string) error {
	f.deleted = append(f.deleted, path)
	return nil
}
func (f *fakeSecretFiles) ClearSecrets(_ context.Context, _ context.Context, _ *api.Project, _ string) error {
	return nil
}

func TestRedactSendOnlyPathAndOnePass(t *testing.T) {
	m := conversationModel()
	c := &recordingClient{}
	m.client = c
	store := &fakeSecretFiles{}
	m.redactStore = store
	m.openRedact()
	m.redactKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("private-secret")})
	m.redactKey(tea.KeyMsg{Type: tea.KeyEnter})
	token := m.input.Value()
	body := m.redactions[token].body
	m.pastes["[Paste fixture]"] = &pastedText{body: "literal " + token}
	draft := "use " + token + " and [Paste fixture]"
	msg := m.sendRedactions(draft)().(redactSent)
	if msg.err != nil || len(c.inputs) != 1 {
		t.Fatal("send failed", msg.err)
	}
	if got := c.inputs[0].Text; got != "use (secret placed at /cxz/secrets/fixture/value) and literal "+token {
		t.Fatal("unsafe replacement", got)
	}
	if len(store.bodies) != 1 || store.bodies[0] != "private-secret" {
		t.Fatal("wrong secret transport")
	}
	m.Update(msg)
	if len(m.redactions) != 0 || strings.Trim(string(body), "\x00") != "" {
		t.Fatal("sent secret retained")
	}
}

func TestRedactArgumentsNeverBecomeChat(t *testing.T) {
	m := conversationModel()
	c := &recordingClient{}
	m.client = c
	m.input.SetValue("/redact accidental-secret")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		cmd()
	}
	if len(c.inputs) != 0 || strings.Contains(m.View(), "accidental-secret") {
		t.Fatal("secret arguments leaked")
	}
}

type failedSecretClient struct{ recordingClient }

func (c *failedSecretClient) Send(context.Context, *api.Input, ...grpc.CallOption) (*api.Receipt, error) {
	return nil, errors.New("failed")
}

func TestRedactSendFailureDeletesFiles(t *testing.T) {
	m := conversationModel()
	m.client = &failedSecretClient{}
	store := &fakeSecretFiles{}
	m.redactStore = store
	m.openRedact()
	m.redactKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("private")})
	m.redactKey(tea.KeyMsg{Type: tea.KeyEnter})
	msg := m.sendRedactions(m.input.Value())().(redactSent)
	if msg.err == nil || len(store.deleted) != 1 {
		t.Fatal("failure retained files")
	}
}

func TestRedactHiddenChipAndDeletion(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("/redact")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.redactDialog == nil {
		t.Fatal("missing modal")
	}
	secret := "private-secret\nline two\nline three\nline four"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(secret), Paste: true})
	if strings.Contains(m.View(), "private-secret") || len(m.pastes) != 0 || m.input.Value() != "" {
		t.Fatal("secret leaked into composer or paste")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	token := m.input.Value()
	if token != "[Redacted]" || m.pastes[token].body != "" || !m.pastes[token].secret {
		t.Fatal("unsafe chip", token)
	}
	if expandPastes(token, m.pastes) != token {
		t.Fatal("normal paste expanded secret")
	}
	m.openPastes()
	if m.pasteDialog != nil {
		t.Fatal("secret preview exposed")
	}
	body := m.redactions[token].body
	m.input.Reset()
	m.pruneRedactions()
	if len(m.redactions) != 0 || strings.Trim(string(body), "\x00") != "" {
		t.Fatal("deleted secret retained")
	}
}
func TestRedactClearCancel(t *testing.T) {
	m := conversationModel()
	m.openRedact()
	m.redactKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("secret")})
	body := m.redactDialog.body
	m.redactKey(tea.KeyMsg{Type: tea.KeyCtrlX})
	if len(m.redactDialog.body) != 0 || strings.Trim(string(body), "\x00") != "" {
		t.Fatal("clear retained secret")
	}
	m.redactKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.redactDialog != nil || len(m.redactions) > 0 {
		t.Fatal("cancel retained secret")
	}
}
func TestRedactMissingContainerFailsBeforeSend(t *testing.T) {
	m := conversationModel()
	c := &recordingClient{}
	m.client = c
	m.openRedact()
	m.redactKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("secret")})
	m.redactKey(tea.KeyMsg{Type: tea.KeyEnter})
	cmd := m.sendRedactions(m.input.Value())
	if cmd == nil {
		t.Fatal("missing command")
	}
	msg := cmd().(redactSent)
	if msg.err == nil || len(c.inputs) > 0 {
		t.Fatal("sent without tmpfs")
	}
	m.Update(msg)
	if len(m.redactions) != 0 {
		t.Fatal("failed send retained secret")
	}
}
