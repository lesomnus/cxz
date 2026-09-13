package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/muesli/termenv"
)

func TestTerminalCursorOnlyMovementAndScroll(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := containerterm.Start(exec.CommandContext(ctx, "sleep", "8"), 100, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := conversationModel()
	m.width, m.height = 100, 60
	p := &terminalPanel{session: s, open: true, focused: true}
	m.terminals = map[string]*terminalPanel{m.current().Id: p}
	m.resize()
	m.input.Cursor.Blink = false
	_, _ = s.Screen.Write([]byte("\x1b[2J\x1b[Habc"))
	before := m.terminalView()
	_, _ = s.Screen.Write([]byte("\x1b[D")) // shell moves cursor without changing any text
	after := m.terminalView()
	if before == after {
		t.Fatal("cursor-only movement did not change frame")
	}
	if !strings.Contains(after, inputCursorStyle.Inline(true).Reverse(true).Render("c")) {
		t.Fatal("cursor does not match composer style")
	}
	for i := 0; i < 100; i++ {
		_, _ = s.Screen.Write([]byte(fmt.Sprintf("\r\nline-%03d", i)))
	}
	m.scrollTerminal(-6)
	if p.scroll == nil {
		t.Fatal("no scrollback")
	}
	frozen := m.terminalView()
	_, _ = s.Screen.Write([]byte("\r\nnew output after scroll"))
	if m.terminalView() != frozen {
		t.Fatal("new output moved history viewport")
	}
	m.terminalMouse(tea.MouseMsg{X: 0, Y: m.height - 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if p.scroll == nil || p.scroll.top != 0 {
		t.Fatal("track start did not reach oldest row")
	}
	m.terminalMouse(tea.MouseMsg{X: m.width - 1, Y: m.height - 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if p.scroll != nil || !strings.Contains(m.terminalView(), "new output after scroll") {
		t.Fatal("track end did not return to live output")
	}
}

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
	if strings.Contains(view, "\x1b[2J") || strings.Contains(view, "\x1b[?1049h") || !strings.Contains(view, terminalFold) || !strings.Contains(view, "한국어 screen") {
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
	if m.terminalHeight() != 26 {
		t.Fatal("not 24 terminal rows")
	}
	lines := strings.Split(m.sessionScreen(), "\n")
	if len(lines) != m.height || !strings.HasSuffix(ansi.Strip(lines[m.terminalTop()]), terminalFold) {
		t.Fatal("header not at expected row", len(lines), m.terminalTop())
	}
	for _, row := range []int{m.terminalTop(), m.height - 2} {
		if got := ansi.Strip(lines[row]); ansi.StringWidth(got) != m.width || strings.HasPrefix(got, " ") || strings.HasSuffix(got, " ") {
			t.Fatalf("terminal separator not full width at %d: %q", row, got)
		}
		m.Update(tea.MouseMsg{X: 1, Y: row, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
		if !p.focused || !p.open {
			t.Fatal("separator click activated a button")
		}
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatal("Ctrl+C escaped to cxz")
	}
	m.Update(tea.MouseMsg{X: m.contentOffset() + 1, Y: m.terminalTop() - 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if p.focused || !p.open {
		t.Fatal("composer click folded panel")
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
	m.Update(tea.MouseMsg{X: m.contentOffset() + m.width - 2, Y: m.terminalTop(), Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
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

func TestTerminalSuccessfulExitFoldsOnlyItsPanel(t *testing.T) {
	for _, code := range []string{"0", "7"} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s, err := containerterm.Start(exec.CommandContext(ctx, "sh", "-c", "exit "+code), 80, 24, nil)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		until := time.Now().Add(3 * time.Second)
		for time.Now().Before(until) {
			if exited, _ := s.Exited(); exited {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		m := conversationModel()
		p := &terminalPanel{session: s, open: true, focused: true}
		m.terminals = map[string]*terminalPanel{m.current().Id: p}
		m.Update(terminalChanged{id: m.current().Id})
		if p.open != (code != "0") || p.focused != (code != "0") {
			t.Fatal("wrong exit policy", code, p.open, p.focused)
		}
		s.Close()
		cancel()
	}
}
