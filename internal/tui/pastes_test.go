package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type pasteClient struct {
	recordingClient
	attachment *api.AttachmentInput
	fail       bool
}

func (c *pasteClient) Attach(_ context.Context, r *api.AttachmentInput, _ ...grpc.CallOption) (*api.Attachment, error) {
	c.attachment = r
	if c.fail {
		return nil, errors.New("offline")
	}
	return &api.Attachment{Path: "/cxz/state/data/sessions/s/attachments/paste.txt"}, nil
}

func TestPasteChipTextAndFile(t *testing.T) {
	m := conversationModel()
	c := &pasteClient{}
	m.client = c
	body := "first\nsecond\nthird\nfourth\n"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(body), Paste: true})
	token := m.input.Value()
	if token == body || !strings.HasPrefix(token, "[Paste ") || expandPastes(token, m.pastes) != body {
		t.Fatal("paste was not losslessly collapsed")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.pasteDialog == nil {
		t.Fatal("no paste modal")
	}
	_, upload := m.Update(questionKeyMsg("f"))
	m.Update(upload())
	if c.attachment.SessionId != "s" || string(c.attachment.Content) != body {
		t.Fatal("wrong attachment target/body")
	}
	if !strings.Contains(decoratePastes(token, m.pastes), "[File ") {
		t.Fatal("file mode not visible")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_, send := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m.Update(send())
	if len(c.inputs) != 1 || strings.Contains(c.inputs[0].Text, "first") || !strings.Contains(c.inputs[0].Text, "/cxz/state/") {
		t.Fatal("file mode sent source or missing path")
	}
	m.input.SetValue(token)
	m.openPastes()
	m.Update(questionKeyMsg("t"))
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_, send = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m.Update(send())
	if c.inputs[1].Text != body {
		t.Fatal("text mode lost newlines")
	}
}

func TestPasteOtherAndSecretExclusion(t *testing.T) {
	m := questionModel()
	m.syncQuestion()
	d := m.questionDialog
	d.row = len(d.questions[0].Options)
	body := "A\nB\nC\nD"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(body), Paste: true})
	if d.texts()[0] != body || !strings.HasPrefix(d.other[0].Value(), "[Paste ") {
		t.Fatal("Other source lost")
	}
	d.questions[0].Secret = true
	before := len(m.pastes)
	if m.capturePaste(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(body), Paste: true}) || len(m.pastes) != before {
		t.Fatal("secret cached")
	}
	m.openPastes()
	if m.pasteDialog != nil {
		t.Fatal("secret attachment allowed")
	}
}

func TestPasteAtomicEditsAndFailure(t *testing.T) {
	p := &pastedText{token: "[Paste abc]", body: "raw"}
	items := map[string]*pastedText{p.token: p}
	for _, after := range []string{"[Paste ab]", "[Paste aXbc]", "[Paste abc\n]"} {
		if !partialPasteEdit(p.token, after, items) {
			t.Fatal(after)
		}
	}
	if partialPasteEdit(p.token, "", items) || partialPasteEdit(p.token, p.token+"x", items) {
		t.Fatal("whole deletion or adjacent editing blocked")
	}
	m := conversationModel()
	m.client = &pasteClient{fail: true}
	m.capturePaste(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat("z", 801)), Paste: true})
	token := m.input.Value()
	m.openPastes()
	_, cmd := m.Update(questionKeyMsg("f"))
	m.Update(cmd())
	if m.pastes[token].file || m.input.Value() != token || !strings.Contains(m.pasteDialog.message, "retained") {
		t.Fatal("failed upload lost source")
	}
}
