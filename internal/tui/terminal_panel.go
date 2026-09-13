package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/containerterm"
)

type terminalPanel struct {
	session                 *containerterm.Session
	open, focused, starting bool
	err                     error
}
type terminalOpened struct {
	id      string
	session *containerterm.Session
	err     error
}
type terminalChanged struct{ id string }

func (m *model) terminal() *terminalPanel {
	if s := m.current(); s != nil {
		return m.terminals[s.Id]
	}
	return nil
}
func (m *model) terminalHeight() int {
	p := m.terminal()
	if p == nil || !p.open || m.projectView || m.accountView || m.workflow != nil {
		return 0
	}
	available := m.height - m.input.Height() - 5 - m.approvalHeight() - 4
	if available < 3 {
		return 0
	}
	return min(25, available) // header + up to 24 PTY rows
}
func (m *model) terminalFocused() bool {
	p := m.terminal()
	return m.terminalHeight() > 0 && p.focused && !m.panelFocus
}
func (m *model) terminalTop() int { return m.height - 1 - m.terminalHeight() }
func (m *model) toggleTerminal() tea.Cmd {
	s := m.current()
	if s == nil || m.projectView || m.accountView || m.workflow != nil {
		return nil
	}
	if m.terminals == nil {
		m.terminals = map[string]*terminalPanel{}
	}
	p := m.terminals[s.Id]
	if p == nil {
		p = &terminalPanel{}
		m.terminals[s.Id] = p
	}
	if p.open && p.focused {
		p.open, p.focused = false, false
		m.input.Focus()
		m.resize()
		return nil
	}
	p.open, p.focused = true, true
	m.panelFocus = false
	m.focusList = false
	m.focusApproval = false
	m.resize()
	if p.starting {
		return nil
	}
	if p.session != nil {
		if exited, _ := p.session.Exited(); !exited {
			return nil
		}
		p.session.Close()
		p.session = nil
	}
	p.starting = true
	p.err = nil
	id, projectID, width, height := s.Id, s.ProjectId, m.width, max(1, m.terminalHeight()-1)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		projects, err := m.client.Projects(ctx, &api.Empty{})
		cancel()
		if err != nil {
			return terminalOpened{id: id, err: err}
		}
		var project *api.Project
		for _, v := range projects.Projects {
			if v.Id == projectID {
				project = v
				break
			}
		}
		if project == nil {
			return terminalOpened{id: id, err: fmt.Errorf("project container unavailable")}
		}
		t, err := containerterm.Open(m.ctx, project, width, height, func() {
			if m.program != nil {
				m.program.Send(terminalChanged{id})
			}
		})
		return terminalOpened{id: id, session: t, err: err}
	}
}

const terminalBack = "[대화로 돌아가기]"
const terminalFold = "[접기 ▾]"

func (m *model) terminalView() string {
	h := m.terminalHeight()
	if h == 0 {
		return ""
	}
	p := m.terminal()
	header := teal.Render(terminalBack) + " " + muted.Render(terminalFold) + "  " + muted.Render("Terminal · Ctrl+` · fold keeps shell")
	if p.focused {
		header = accent.Render(terminalBack) + " " + teal.Render(terminalFold) + "  " + accent.Render("Terminal · focused · Ctrl+`")
	}
	body := ""
	if p.starting {
		body = "  Preparing container terminal…"
	} else if p.err != nil {
		body = "  " + safeText(p.err.Error()) + " · fold and reopen to retry"
	} else if p.session != nil {
		body = p.session.Screen.Render()
		if exited, err := p.session.Exited(); exited {
			header += " · exited"
			if err != nil {
				header += " (error)"
			}
		}
	}
	lines := strings.Split(body, "\n")
	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], m.width, "")
	}
	return clip(header, m.width) + "\n" + strings.Join(lines[:h-1], "\n")
}

func (m *model) terminalMouse(v tea.MouseMsg) bool {
	if m.terminalHeight() == 0 {
		return false
	}
	x := v.X - m.contentOffset()
	p := m.terminal()
	if x < 0 || x >= m.width {
		return false
	}
	top := m.terminalTop()
	if v.Y == top {
		if v.Action == tea.MouseActionPress && v.Button == tea.MouseButtonLeft {
			if x < ansi.StringWidth(terminalBack) {
				p.focused = false
				m.panelFocus, m.focusList, m.focusApproval = false, false, false
				m.input.Focus()
			} else if x < ansi.StringWidth(terminalBack)+1+ansi.StringWidth(terminalFold) {
				p.open, p.focused = false, false
				m.panelFocus, m.focusList, m.focusApproval = false, false, false
				m.input.Focus()
				m.resize()
			}
		}
		return true
	}
	if v.Y > top && v.Y < m.height-1 {
		if v.Action == tea.MouseActionPress {
			p.focused = true
			m.panelFocus = false
		}
		if p.session != nil {
			mouse := uv.Mouse{X: x, Y: v.Y - top - 1}
			buttons := map[tea.MouseButton]uv.MouseButton{tea.MouseButtonLeft: uv.MouseLeft, tea.MouseButtonRight: uv.MouseRight, tea.MouseButtonMiddle: uv.MouseMiddle, tea.MouseButtonWheelUp: uv.MouseWheelUp, tea.MouseButtonWheelDown: uv.MouseWheelDown}
			mouse.Button = buttons[v.Button]
			if v.Shift {
				mouse.Mod |= uv.ModShift
			}
			if v.Alt {
				mouse.Mod |= uv.ModAlt
			}
			if v.Ctrl {
				mouse.Mod |= uv.ModCtrl
			}
			var event uv.MouseEvent = uv.MouseClickEvent(mouse)
			if v.Action == tea.MouseActionRelease {
				event = uv.MouseReleaseEvent(mouse)
			} else if v.Action == tea.MouseActionMotion {
				event = uv.MouseMotionEvent(mouse)
			} else if v.Button == tea.MouseButtonWheelUp || v.Button == tea.MouseButtonWheelDown {
				event = uv.MouseWheelEvent(mouse)
			}
			terminal := p.session
			terminal.Queue(func() { terminal.Screen.SendMouse(event) })
		}
		return true
	}
	if v.Action == tea.MouseActionPress && v.Button == tea.MouseButtonLeft && v.Y >= top-m.input.Height()-2 && v.Y < top {
		p.focused = false
		m.panelFocus, m.focusList, m.focusApproval = false, false, false
		m.input.Focus()
		return true
	}
	return false
}

func (m *model) terminalKey(k tea.KeyMsg) {
	p := m.terminal()
	if p == nil || p.session == nil {
		return
	}
	ok := false
	if k.Type == tea.KeyRunes && !k.Alt {
		ok = p.session.Text(string(k.Runes), k.Paste)
	} else if key, valid := terminalKeyEvent(k); valid {
		ok = p.session.Key(key)
	}
	if !ok {
		m.notice = "Terminal input unavailable or busy"
	}
}

func terminalKeyEvent(k tea.KeyMsg) (uv.KeyPressEvent, bool) {
	key := uv.KeyPressEvent{}
	name := k.String()
	for {
		switch {
		case strings.HasPrefix(name, "ctrl+"):
			key.Mod |= uv.ModCtrl
			name = strings.TrimPrefix(name, "ctrl+")
		case strings.HasPrefix(name, "alt+"):
			key.Mod |= uv.ModAlt
			name = strings.TrimPrefix(name, "alt+")
		case strings.HasPrefix(name, "shift+"):
			key.Mod |= uv.ModShift
			name = strings.TrimPrefix(name, "shift+")
		default:
			goto mapped
		}
	}
mapped:
	codes := map[string]rune{"enter": uv.KeyEnter, "tab": uv.KeyTab, "backspace": uv.KeyBackspace, "esc": uv.KeyEscape, "up": uv.KeyUp, "down": uv.KeyDown, "left": uv.KeyLeft, "right": uv.KeyRight, "home": uv.KeyHome, "end": uv.KeyEnd, "pgup": uv.KeyPgUp, "pgdown": uv.KeyPgDown, "delete": uv.KeyDelete, "insert": uv.KeyInsert, " ": uv.KeySpace, "space": uv.KeySpace, "@": uv.KeySpace}
	for i := 1; i <= 20; i++ {
		codes[fmt.Sprintf("f%d", i)] = uv.KeyF1 + rune(i-1)
	}
	if code, ok := codes[name]; ok {
		key.Code = code
		return key, true
	}
	if r := []rune(name); len(r) == 1 {
		key.Code = r[0]
		return key, true
	}
	return key, false
}
