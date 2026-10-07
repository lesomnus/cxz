package tui

import (
	"bytes"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/bed"
	"reflect"
	"strings"
	"testing"
)

func multiComposerModel(t *testing.T, text string, positions ...int) *model {
	t.Helper()
	m := conversationModel()
	m.input.SetValue(text)
	m.prepareEditor()
	var ss []bed.Selection
	for _, p := range positions {
		ss = append(ss, bed.Selection{Anchor: p, Head: p, Column: -1})
	}
	if err := m.input.SetSelections(ss, 0); err != nil {
		t.Fatal(err)
	}
	m.resize()
	return m
}
func TestComposerMultiGesturesTypingUndoAndSend(t *testing.T) {
	m := multiComposerModel(t, "one\ntwo", 0)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlDown, Alt: true})
	if !m.multiComposer() {
		t.Fatal("key did not add cursor")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if m.input.Value() != "Xone\nXtwo" {
		t.Fatal(m.input.Value())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if m.input.Value() != "one\ntwo" || !m.multiComposer() {
		t.Fatal("undo")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("send")
	}
	m.Update(cmd())
	c := m.client.(*recordingClient)
	if len(c.inputs) != 1 || c.inputs[0].Text != "Xone\nXtwo" {
		t.Fatal(c.inputs)
	}
	if m.input.Value() != "" || m.multiComposer() {
		t.Fatal("send did not clear")
	}
}
func TestComposerMultiMouseResizeAndEscape(t *testing.T) {
	m := multiComposerModel(t, "one\ntwo", 0)
	top := m.height - m.input.Height() - 2 - m.terminalHeight()
	m.Update(tea.MouseMsg{X: m.contentOffset() + 4, Y: top + 1, Alt: true, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.multiComposer() {
		t.Fatal("mouse did not add")
	}
	before := m.input.Selections()
	m.Update(tea.WindowSizeMsg{Width: 65, Height: 18})
	if !reflect.DeepEqual(before, m.input.Selections()) {
		t.Fatal("resize dropped cursors")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.multiComposer() || m.interruptKey != "" {
		t.Fatal("escape interrupted")
	}
}
func TestComposerMultiChipCaptureDeleteUndoAndExpansion(t *testing.T) {
	m := multiComposerModel(t, "a\nb", 0, 2)
	body := "one\ntwo\nthree\nfour"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(body), Paste: true})
	if len(m.pastes) != 1 || !m.multiComposer() {
		t.Fatal("paste bypassed chip", m.input.Value())
	}
	token := ""
	for k := range m.pastes {
		token = k
	}
	if m.input.Value() != token+"a\n"+token+"b" {
		t.Fatal(m.input.Value())
	}
	expanded := expandPastes(m.input.Value(), m.pastes)
	if expanded != body+"a\n"+body+"b" {
		t.Fatal(expanded)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.input.Value() != "a\nb" {
		t.Fatal("chip split", m.input.Value())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if expandPastes(m.input.Value(), m.pastes) != expanded {
		t.Fatal("undo lost payload")
	}
	m.openPastes()
	if m.pasteDialog == nil {
		t.Fatal("preview")
	}
	m.pasteKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if !strings.HasPrefix(m.input.Value(), body) || !m.multiComposer() {
		t.Fatal("expand lost cursors", m.input.Value())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if expandPastes(m.input.Value(), m.pastes) != expanded {
		t.Fatal("expand undo lost payload")
	}
}
func TestComposerMultiClipboardHintsAndDetach(t *testing.T) {
	m := multiComposerModel(t, "@seal\n@seal", 0, 6)
	var out bytes.Buffer
	m.cursorOutput = &cursorWriter{out: &out}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !strings.Contains(out.String(), ansi.SetSystemClipboard("@seal\n@seal")) {
		t.Fatal("copy")
	}
	if token, _ := m.mentionHints(); token != nil {
		t.Fatal("mention active")
	}
	if _, _, ok := m.pathContext(); ok {
		t.Fatal("path hints active")
	}
	if len(m.commandHints()) != 0 || m.inlineContext() != nil {
		t.Fatal("single target hints active")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("detach")
	}
}
func TestComposerMultiNewlineAndIndent(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyEnter, Alt: true}, {Type: tea.KeyCtrlJ}} {
		m := multiComposerModel(t, "a\nb", 1, 3)
		m.Update(key)
		if m.input.Value() != "a\n\nb\n" || !m.multiComposer() {
			t.Fatal(key.String(), m.input.Value())
		}
	}
	m := multiComposerModel(t, "a\nb", 0, 2)
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.input.Value() != "    a\n    b" {
		t.Fatal(m.input.Value())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if m.input.Value() != "a\nb" || !m.multiComposer() {
		t.Fatal("undo")
	}
}
