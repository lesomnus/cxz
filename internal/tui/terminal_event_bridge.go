package tui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
)

// Keep modifier information until we reach the application's key bindings.
// Bubble Tea v1's native Windows decoder discards Ctrl on VK_RETURN, while its
// byte-stream fallback decodes Unicode using the system ANSI code page.
func terminalEventMessages(event uv.Event) []tea.Msg {
	switch e := event.(type) {
	case uv.MultiEvent:
		var msgs []tea.Msg
		for _, part := range e {
			msgs = append(msgs, terminalEventMessages(part)...)
		}
		return msgs
	case uv.KeyPressEvent:
		if key, ok := terminalEventKey(e); ok {
			return []tea.Msg{key}
		}
	case uv.PasteEvent:
		return []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(e.Content), Paste: true}}
	case uv.WindowSizeEvent:
		return []tea.Msg{tea.WindowSizeMsg{Width: e.Width, Height: e.Height}}
	case uv.FocusEvent:
		return []tea.Msg{tea.FocusMsg{}}
	case uv.BlurEvent:
		return []tea.Msg{tea.BlurMsg{}}
	case uv.MouseEvent:
		mouse := e.Mouse()
		buttons := map[uv.MouseButton]tea.MouseButton{
			uv.MouseNone: tea.MouseButtonNone, uv.MouseLeft: tea.MouseButtonLeft,
			uv.MouseMiddle: tea.MouseButtonMiddle, uv.MouseRight: tea.MouseButtonRight,
			uv.MouseWheelUp: tea.MouseButtonWheelUp, uv.MouseWheelDown: tea.MouseButtonWheelDown,
			uv.MouseWheelLeft: tea.MouseButtonWheelLeft, uv.MouseWheelRight: tea.MouseButtonWheelRight,
			uv.MouseBackward: tea.MouseButtonBackward, uv.MouseForward: tea.MouseButtonForward,
		}
		button, ok := buttons[mouse.Button]
		if !ok {
			return nil
		}
		m := tea.MouseMsg{X: mouse.X, Y: mouse.Y, Button: button, Action: tea.MouseActionPress,
			Ctrl: mouse.Mod&uv.ModCtrl != 0, Alt: mouse.Mod&uv.ModAlt != 0, Shift: mouse.Mod&uv.ModShift != 0}
		types := map[tea.MouseButton]tea.MouseEventType{
			tea.MouseButtonLeft: tea.MouseLeft, tea.MouseButtonMiddle: tea.MouseMiddle, tea.MouseButtonRight: tea.MouseRight,
			tea.MouseButtonWheelUp: tea.MouseWheelUp, tea.MouseButtonWheelDown: tea.MouseWheelDown,
			tea.MouseButtonWheelLeft: tea.MouseWheelLeft, tea.MouseButtonWheelRight: tea.MouseWheelRight,
			tea.MouseButtonBackward: tea.MouseBackward, tea.MouseButtonForward: tea.MouseForward,
		}
		m.Type = types[button]
		switch e.(type) {
		case uv.MouseReleaseEvent:
			m.Type, m.Action = tea.MouseRelease, tea.MouseActionRelease
		case uv.MouseMotionEvent:
			m.Type, m.Action = tea.MouseMotion, tea.MouseActionMotion
		}
		return []tea.Msg{m}
	}
	return nil
}

var terminalKeyTypes = func() map[string]tea.KeyType {
	m := map[string]tea.KeyType{}
	for code := tea.KeyF20; code <= tea.KeyRunes; code++ {
		if name := (tea.KeyMsg{Type: code}).String(); name != "" {
			m[name] = code
		}
	}
	for code := tea.KeyType(0); code <= 127; code++ {
		if name := (tea.KeyMsg{Type: code}).String(); name != "" {
			m[name] = code
		}
	}
	return m
}()

func terminalEventKey(key uv.KeyPressEvent) (tea.KeyMsg, bool) {
	key.Mod &^= uv.ModCapsLock | uv.ModNumLock | uv.ModScrollLock
	if key.Text != "" {
		// Includes IME text and AltGr, whose Ctrl+Alt state is not a shortcut.
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key.Text)}, true
	}
	if key.Mod & ^(uv.ModCtrl|uv.ModAlt|uv.ModShift) != 0 {
		return tea.KeyMsg{}, false
	}
	alt := key.Mod&uv.ModAlt != 0
	if key.Code == uv.KeyEnter || key.Code == uv.KeyKpEnter {
		if key.Mod == uv.ModCtrl {
			return tea.KeyMsg{Type: tea.KeyCtrlS}, true
		}
		return tea.KeyMsg{Type: tea.KeyEnter, Alt: alt}, true
	}
	if key.Code == uv.KeyBackspace && key.Mod&uv.ModCtrl != 0 {
		return tea.KeyMsg{Type: tea.KeyCtrlW, Alt: alt}, true
	}
	if key.Code == '`' && key.Mod == uv.ModCtrl {
		return tea.KeyMsg{Type: tea.KeyF20}, true
	}
	if key.Mod&uv.ModCtrl != 0 {
		code := unicode.ToLower(key.Code)
		switch {
		case code >= 'a' && code <= 'z':
			return tea.KeyMsg{Type: tea.KeyType(code - 'a' + 1), Alt: alt}, true
		case code == ' ' || code == '@' || code == '2':
			return tea.KeyMsg{Type: tea.KeyCtrlAt, Alt: alt}, true
		case code >= '[' && code <= '_':
			return tea.KeyMsg{Type: tea.KeyType(code - '@'), Alt: alt}, true
		case code == '/':
			return tea.KeyMsg{Type: tea.KeyCtrlUnderscore, Alt: alt}, true
		case code >= '3' && code <= '7':
			return tea.KeyMsg{Type: tea.KeyType(code - '3' + 27), Alt: alt}, true
		case code == '8' || code == '?':
			return tea.KeyMsg{Type: tea.KeyBackspace, Alt: alt}, true
		case code == '~':
			return tea.KeyMsg{Type: tea.KeyCtrlCaret, Alt: alt}, true
		}
	}
	key.Mod &^= uv.ModAlt
	if kind, ok := terminalKeyTypes[key.String()]; ok {
		return tea.KeyMsg{Type: kind, Alt: alt}, true
	}
	if key.Mod&uv.ModCtrl == 0 && unicode.IsPrint(key.Code) {
		text := string(key.Code)
		if key.Mod&uv.ModShift != 0 {
			text = strings.ToUpper(text)
		}
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Alt: alt}, true
	}
	// Preserve the old console behavior for modifiers Bubble Tea cannot express
	// on special keys (for example Shift+Backspace or Ctrl+Delete).
	if !unicode.IsPrint(key.Code) {
		key.Mod = 0
		if kind, ok := terminalKeyTypes[key.String()]; ok {
			return tea.KeyMsg{Type: kind, Alt: alt}, true
		}
	}
	return tea.KeyMsg{}, false
}
