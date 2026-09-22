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
	scroll                  *terminalScroll
}
type terminalScroll struct {
	lines []uv.Line
	top   int
}
type terminalOpened struct {
	id      string
	session *containerterm.Session
	err     error
}
type terminalChanged struct{ id string }

func (m *model) closeSuccessfulTerminal(id string) {
	p := m.terminals[id]
	if p == nil || p.session == nil {
		return
	}
	if exited, err := p.session.Exited(); exited && err == nil {
		focused := p.focused
		p.open, p.focused = false, false
		if s := m.current(); s != nil && s.Id == id {
			if focused && !m.panelFocus {
				m.input.Focus()
			}
			m.resize()
		}
	}
}

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
	available := m.height - m.input.Height() - 5 - m.approvalHeight() - m.errorHeight() - 4
	if available < 4 {
		return 0
	}
	return min(26, available) // two rules + up to 24 PTY rows
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
	p.scroll = nil
	id, projectID, width, height := s.Id, s.ProjectId, m.width, max(1, m.terminalHeight()-2)
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
		t, err := containerterm.Open(m.contextFor(projectID), m.localProject(project), width, height, func() {
			if m.program != nil {
				m.program.Send(terminalChanged{id})
			}
		})
		return terminalOpened{id: id, session: t, err: err}
	}
}

const terminalFold = "[Collapse ▾]"

func (m *model) terminalView() string {
	h := m.terminalHeight()
	if h == 0 {
		return ""
	}
	p := m.terminal()
	header := teal.Render(strings.Repeat("─", max(0, m.width-ansi.StringWidth(terminalFold)))) + teal.Render(terminalFold)
	body := ""
	if p.starting {
		body = "  Preparing container terminal…"
	} else if p.err != nil {
		body = "  " + safeText(p.err.Error()) + " · fold and reopen to retry"
	} else if p.session != nil {
		frame := p.session.Capture(false)
		rows := frame.Lines
		if p.scroll != nil {
			rows = p.scroll.lines[min(p.scroll.top, len(p.scroll.lines)):]
		}
		lines := make([]string, min(h-2, len(rows)))
		for y := range lines {
			lines[y] = rows[y].Render()
		}
		if p.scroll == nil && m.terminalFocused() && frame.CursorVisible && !m.input.Cursor.Blink {
			x, y := frame.Cursor.X, frame.Cursor.Y
			if y >= 0 && y < len(lines) && x >= 0 && x < m.width && x < len(rows[y]) {
				cell := rows[y][x]
				text := cell.Content
				if text == "" {
					text = " "
				}
				line := lines[y]
				line += strings.Repeat(" ", max(0, x+max(1, cell.Width)-ansi.StringWidth(line)))
				lines[y] = ansi.Cut(line, 0, x) + inputCursorStyle.Inline(true).Reverse(true).Render(text) + ansi.Cut(line, x+max(1, cell.Width), m.width)
			}
		}
		body = strings.Join(lines, "\n")
	}
	lines := strings.Split(body, "\n")
	for len(lines) < h-2 {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], m.width, "")
	}
	return clip(header, m.width) + "\n" + strings.Join(lines[:h-2], "\n") + "\n" + m.terminalTrack()
}

func (m *model) terminalTrack() string {
	position := max(0, m.width-1)
	if p := m.terminal(); p != nil && p.scroll != nil {
		end := max(1, len(p.scroll.lines)-(m.terminalHeight()-2))
		position = min(m.width-1, p.scroll.top*(m.width-1)/end)
	}
	return muted.Render(strings.Repeat("─", max(0, position))) + muted.Render("◆︎") + zeroStyle.Render(strings.Repeat("─", max(0, m.width-position-1)))
}
func (m *model) scrollTerminal(delta int) {
	p := m.terminal()
	if p == nil || p.session == nil {
		return
	}
	if p.scroll == nil {
		if delta >= 0 {
			return
		}
		rows := p.session.Capture(true).Lines
		p.scroll = &terminalScroll{lines: rows, top: max(0, len(rows)-(m.terminalHeight()-2))}
	}
	end := max(0, len(p.scroll.lines)-(m.terminalHeight()-2))
	p.scroll.top = max(0, min(end, p.scroll.top+delta))
	if p.scroll.top == end {
		p.scroll = nil
	}
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
			if x >= m.width-ansi.StringWidth(terminalFold) {
				p.open, p.focused = false, false
				m.panelFocus, m.focusList, m.focusApproval = false, false, false
				m.input.Focus()
				m.resize()
			}
		}
		return true
	}
	if v.Y == m.height-2 {
		if v.Button == tea.MouseButtonWheelUp {
			m.scrollTerminal(-3)
		} else if v.Button == tea.MouseButtonWheelDown {
			m.scrollTerminal(3)
		} else if v.Button == tea.MouseButtonLeft && (v.Action == tea.MouseActionPress || v.Action == tea.MouseActionMotion) {
			m.scrollTerminal(-1)
			if p.scroll != nil {
				end := max(0, len(p.scroll.lines)-(m.terminalHeight()-2))
				p.scroll.top = x * end / max(1, m.width-1)
				if x == m.width-1 {
					p.scroll = nil
				}
			}
		}
		return true
	}
	if v.Y > top && v.Y < m.height-2 {
		if p.session != nil && (!p.session.Screen.IsAltScreen() || v.Shift || p.scroll != nil) {
			if v.Button == tea.MouseButtonWheelUp {
				m.scrollTerminal(-3)
				return true
			}
			if v.Button == tea.MouseButtonWheelDown {
				m.scrollTerminal(3)
				return true
			}
		}
		if v.Action == tea.MouseActionPress {
			p.focused = true
			m.panelFocus = false
		}
		if p.session != nil && p.scroll == nil {
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
	if k.Type == tea.KeyPgUp && k.Alt {
		m.scrollTerminal(-(m.terminalHeight() - 2))
		return
	}
	if k.Type == tea.KeyPgDown && k.Alt {
		m.scrollTerminal(m.terminalHeight() - 2)
		return
	}
	if p.scroll != nil && k.Type == tea.KeyCtrlEnd {
		p.scroll = nil
		return
	}
	p.scroll = nil // typing returns to the live shell, never into a historical row
	m.input.Cursor.Blink = false
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
