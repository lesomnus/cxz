package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
	"google.golang.org/grpc"
)

func TestFileChipLabelStableAcrossCursorBlink(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	for _, width := range []int{70, 24} {
		m := conversationModel()
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\nb\nc\nd"), Paste: true})
		token := m.input.Value()
		m.pastes[token].file = true
		m.input.SetWidth(width)
		for pos := 0; pos < 15; pos++ {
			m.input.SetCursor(pos)
			for _, blink := range []bool{false, true} {
				m.input.Cursor.Blink = blink
				view := m.input.View()
				got := m.decorateInputPastes(view)
				plain := ansi.Strip(got)
				if strings.Contains(plain, "[Paste") || !strings.Contains(plain, "[File  ") {
					t.Fatalf("width=%d cursor=%d blink=%v: %q", width, pos, blink, got)
				}
				if ansi.StringWidth(view) != ansi.StringWidth(got) || m.input.Value() != token {
					t.Fatal("changed layout or underlying chip")
				}
			}
		}
	}
}

func TestFileChipDecorationPreservesEscapesAndUnicode(t *testing.T) {
	token := "[Paste abcd1234 · 4L · 8B]"
	items := map[string]*pastedText{token: {token: token, file: true}}
	input := "한글 e\u0301\n\x1b[32m[Pa\x1b[7ms\x1b[27mte abcd1234 · 4L · 8B]\x1b[0m"
	want := "한글 e\u0301\n\x1b[32m[Fi\x1b[7ml\x1b[27me  abcd1234 · 4L · 8B]\x1b[0m"
	if got := decoratePastes(input, items); got != want {
		t.Fatalf("got %q; want %q", got, want)
	}
}

func TestSelectedChipHighlightPreservesLayout(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	for _, width := range []int{70, 24} {
		m := conversationModel()
		m.input.SetWidth(width)
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\nb\nc\nd"), Paste: true})
		m.Update(tea.KeyMsg{Type: tea.KeyLeft})
		m.input.SetWidth(width)
		before := m.input.View()
		after := m.decorateInputPastes(before)
		if ansi.Strip(before) != ansi.Strip(after) {
			t.Fatal("highlight changed layout")
		}
		if !strings.Contains(after, "48;2;174;255;152") {
			t.Fatalf("missing chip highlight at width %d: %q", width, after)
		}
	}
}

func TestChipAtomicNavigationAndMenu(t *testing.T) {
	m := conversationModel()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\nb\nc\nd"), Paste: true})
	token := m.input.Value()
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.pasteSelection == nil {
		t.Fatal("left must select chip")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.pasteDialog == nil || m.input.Value() != token {
		t.Fatal("enter must open chip menu without newline")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.pasteSelection != nil {
		t.Fatal("second arrow must exit chip")
	}
	_, pos, _, _, _ := m.chipInput()
	if pos != len([]rune(token)) {
		t.Fatal("did not exit at end")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.input.Value() != "" {
		t.Fatal("backspace must remove whole chip")
	}
}

func TestChipOtherDeleteAndLiteralInput(t *testing.T) {
	m := questionModel()
	m.syncQuestion()
	d := m.questionDialog
	d.row = len(d.questions[0].Options)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\nb\nc\nd"), Paste: true})
	token := d.other[0].Value()
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m.Update(questionKeyMsg("t"))
	if d.other[0].Value() != token {
		t.Fatal("selected chip t must set text mode without typing")
	}
	m.Update(questionKeyMsg("d"))
	if d.other[0].Value() != "" {
		t.Fatal("d must remove whole Other chip")
	}
	m.Update(questionKeyMsg("t"))
	if d.other[0].Value() != "t" {
		t.Fatal("unselected t must remain literal")
	}
}

func TestSelectedChipDirectFileAndCancel(t *testing.T) {
	for _, cancel := range []string{"", "t", "d"} {
		m := conversationModel()
		m.client = &pasteClient{}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\nb\nc\nd"), Paste: true})
		token := m.input.Value()
		m.Update(tea.KeyMsg{Type: tea.KeyLeft})
		_, upload := m.Update(questionKeyMsg("f"))
		if upload == nil || m.pasteDialog != nil {
			t.Fatal("f must upload directly without opening a preview")
		}
		if cancel != "" {
			m.Update(questionKeyMsg(cancel))
		}
		m.Update(upload())
		if m.pastes[token].file != (cancel == "") {
			t.Fatal("late upload changed cancelled file mode")
		}
	}
}

func TestDeletedChipsDoNotReappearInPreview(t *testing.T) {
	for _, method := range []string{"d", "backspace", "menu"} {
		m := conversationModel()
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\nb\nc\nd"), Paste: true})
		if method == "menu" {
			m.openPastes()
			m.Update(questionKeyMsg("d"))
		} else {
			m.Update(tea.KeyMsg{Type: tea.KeyLeft})
			if method == "d" {
				m.Update(questionKeyMsg("d"))
			} else {
				m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
			}
		}
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
		if m.pasteDialog != nil {
			t.Fatalf("%s resurrected a deleted chip", method)
		}
	}
}

func TestSelectedChipPastedShortcutIsLiteral(t *testing.T) {
	m := conversationModel()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\nb\nc\nd"), Paste: true})
	token := m.input.Value()
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d"), Paste: true})
	if m.input.Value() != "d"+token {
		t.Fatal("pasted d executed deletion")
	}
}

func TestDirectChipUploadFailure(t *testing.T) {
	m := conversationModel()
	m.client = &pasteClient{fail: true}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\nb\nc\nd"), Paste: true})
	token := m.input.Value()
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	_, upload := m.Update(questionKeyMsg("f"))
	m.Update(upload())
	if m.input.Value() != token || m.pastes[token].file || !strings.Contains(m.notice, "Upload failed") {
		t.Fatal("upload failure must preserve draft and report error")
	}
}

func TestChipMultilineUnicodeDeletion(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("한글\n앞 ")
	m.input.CursorEnd()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\nb\nc\nd"), Paste: true})
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.input.Value() != "한글\n앞 " {
		t.Fatalf("lost surrounding draft: %q", m.input.Value())
	}
}

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
