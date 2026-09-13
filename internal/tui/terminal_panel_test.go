package tui

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/internal/containerterm"
)

func TestFoldRetainsPTYAndContainsScreenControl(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := containerterm.Start(exec.CommandContext(ctx, "sh"), 100, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := conversationModel()
	m.width, m.height = 100, 60
	p := &terminalPanel{session: s, open: true, focused: true}
	m.terminals = map[string]*terminalPanel{m.current().Id: p}
	m.resize()
	m.render()
	s.Text("value=kept\r", false)
	m.Update(tea.KeyMsg{Type: tea.KeyF20})
	if exited, _ := s.Exited(); exited {
		t.Fatal("fold killed PTY")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF20})
	if p.session != s {
		t.Fatal("reopen replaced PTY")
	}
	s.Text("printf 'state-%s\\n' \"$value\"\r", false)
	until := time.Now().Add(3 * time.Second)
	for !strings.Contains(ansi.Strip(s.Screen.Render()), "state-kept") && time.Now().Before(until) {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(ansi.Strip(s.Screen.Render()), "state-kept") {
		t.Fatal("shell state lost")
	}
	_, _ = s.Screen.Write([]byte("\x1b[?1049h\x1b[2J\x1b[H한국어 screen"))
	view := m.sessionScreen()
	if strings.Contains(view, "\x1b[2J") || strings.Contains(view, "\x1b[?1049h") || !strings.Contains(view, terminalBack) || !strings.Contains(view, "한국어 screen") {
		t.Fatal("terminal control escaped its panel", view)
	}
}

func TestTerminalPanelFocusAndLayout(t *testing.T) {
	m := conversationModel()
	m.height = 60
	m.width = 100
	p := &terminalPanel{open: true, focused: true, starting: true}
	m.terminals = map[string]*terminalPanel{m.current().Id: p}
	m.resize()
	m.render()
	if m.terminalHeight() != 25 {
		t.Fatal("not 24 terminal rows")
	}
	lines := strings.Split(m.sessionScreen(), "\n")
	if len(lines) != m.height || !strings.Contains(lines[m.terminalTop()], terminalBack) {
		t.Fatal("header not at expected row", len(lines), m.terminalTop())
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatal("Ctrl+C escaped to cxz")
	}
	m.Update(tea.MouseMsg{X: m.contentOffset() + 1, Y: m.terminalTop(), Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if p.focused || !p.open {
		t.Fatal("return button folded panel")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF20})
	if !p.focused || !p.open {
		t.Fatal("toggle did not focus")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF20})
	if p.focused || p.open {
		t.Fatal("toggle did not fold")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF20})
	m.Update(tea.MouseMsg{X: m.contentOffset() + ansi.StringWidth(terminalBack) + 2, Y: m.terminalTop(), Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if p.open || p.focused {
		t.Fatal("fold button failed")
	}
	for _, h := range []int{14, 20, 30, 60} {
		m.height = h
		p.open = true
		m.resize()
		m.render()
		if got := len(strings.Split(m.sessionScreen(), "\n")); got > h {
			t.Fatal("panel overflow", h, got)
		}
	}
}
func TestTerminalKeyEncoding(t *testing.T) {
	for _, tc := range []struct {
		in   tea.KeyMsg
		code rune
		mod  uv.KeyMod
	}{{tea.KeyMsg{Type: tea.KeyCtrlC}, 'c', uv.ModCtrl}, {tea.KeyMsg{Type: tea.KeyUp}, uv.KeyUp, 0}, {tea.KeyMsg{Type: tea.KeyEnter}, uv.KeyEnter, 0}, {tea.KeyMsg{Type: tea.KeyShiftTab}, uv.KeyTab, uv.ModShift}} {
		key, ok := terminalKeyEvent(tc.in)
		if !ok || key.Code != tc.code || key.Mod != tc.mod {
			t.Fatal(tc, key)
		}
	}
}
