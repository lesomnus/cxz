package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
)

func win32Key(vk int, char rune, modifiers, down, repeat int) string {
	return fmt.Sprintf("\x1b[%d;0;%d;%d;%d;%d_", vk, char, down, modifiers, repeat)
}

func decodedTerminalMessages(t *testing.T, input io.Reader) []tea.Msg {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events := make(chan uv.Event)
	done := make(chan struct{})
	var messages []tea.Msg
	go func() {
		defer close(done)
		for event := range events {
			messages = append(messages, terminalEventMessages(event)...)
		}
	}()
	err := uv.NewTerminalReader(input, "").StreamEvents(ctx, events)
	close(events)
	<-done
	if err != nil {
		t.Fatal(err)
	}
	return messages
}

func TestWindowsNativeKeyModifiersReachBindings(t *testing.T) {
	for _, tc := range []struct{ name, wire, key string }{
		{"enter", win32Key(13, '\r', 0, 1, 1), "enter"},
		{"left ctrl enter", win32Key(13, '\n', 8, 1, 1), "ctrl+s"},
		{"right ctrl enter", win32Key(13, '\n', 4, 1, 1), "ctrl+s"},
		{"keypad ctrl enter with locks", win32Key(13, '\n', 8|256|128|32|64, 1, 1), "ctrl+s"},
		{"shift enter", win32Key(13, '\r', 16, 1, 1), "enter"},
		{"alt enter", win32Key(13, '\r', 2, 1, 1), "alt+enter"},
		{"ctrl s", win32Key('S', 19, 8, 1, 1), "ctrl+s"},
		{"ctrl j remains newline", win32Key('J', '\n', 8, 1, 1), "ctrl+j"},
		{"ctrl left", win32Key(37, 0, 8|256, 1, 1), "ctrl+left"},
		{"ctrl shift right", win32Key(39, 0, 8|16|256, 1, 1), "ctrl+shift+right"},
		{"shift tab", win32Key(9, '\t', 16, 1, 1), "shift+tab"},
		{"ctrl backspace", win32Key(8, 127, 8, 1, 1), "ctrl+w"},
		{"ctrl grave", win32Key(192, 0, 8, 1, 1), "f20"},
		{"unicode", win32Key(0, '한', 0, 1, 1), "한"},
		{"alt gr", win32Key('Q', '@', 8|1, 1, 1), "@"},
		{"f9", win32Key(120, 0, 0, 1, 1), "f9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msgs := decodedTerminalMessages(t, strings.NewReader(tc.wire))
			if len(msgs) != 1 {
				t.Fatal(msgs)
			}
			key, ok := msgs[0].(tea.KeyMsg)
			if !ok || key.String() != tc.key {
				t.Fatalf("want %s, got %#v", tc.key, msgs[0])
			}
		})
	}
}

func TestWindowsCtrlEnterSendsWhilePlainEnterEdits(t *testing.T) {
	m := conversationModel()
	client := m.client.(*recordingClient)
	m.input.SetValue("message")
	plain := decodedTerminalMessages(t, strings.NewReader(win32Key(13, '\r', 0, 1, 1)))
	if _, cmd := m.Update(plain[0]); cmd != nil || m.input.Value() != "message\n" {
		t.Fatal("Enter no longer inserts a newline")
	}
	submit := decodedTerminalMessages(t, strings.NewReader(win32Key(13, '\n', 8, 1, 1)))
	_, cmd := m.Update(submit[0])
	if cmd == nil {
		t.Fatal("Ctrl+Enter did not submit")
	}
	cmd()
	if len(client.inputs) != 1 {
		t.Fatal("Ctrl+Enter did not reach Send", client.inputs)
	}
}

func TestWindowsUnicodePasteRepeatAndKeyRelease(t *testing.T) {
	wire := win32Key(13, '\n', 8, 0, 1) + win32Key('A', 'a', 0, 1, 3) +
		win32Key(0, 0xd83d, 0, 1, 1) + win32Key(0, 0xde42, 0, 1, 1)
	msgs := decodedTerminalMessages(t, strings.NewReader(wire))
	var text strings.Builder
	for _, msg := range msgs {
		key := msg.(tea.KeyMsg)
		if key.Type != tea.KeyRunes {
			t.Fatal("key release became an action", key)
		}
		text.WriteString(string(key.Runes))
	}
	if text.String() != "aaa🙂" {
		t.Fatal(text.String())
	}
	paste := "한글🙂\n\x1b[13;5u"
	wire = "\x1b[200~" + paste + "\x1b[201~"
	msgs = decodedTerminalMessages(t, strings.NewReader(wire))
	if len(msgs) != 1 {
		t.Fatal(msgs)
	}
	key := msgs[0].(tea.KeyMsg)
	if !key.Paste || string(key.Runes) != paste {
		t.Fatal(key)
	}
}

func TestWindowsMouseAndResizeBridge(t *testing.T) {
	msgs := decodedTerminalMessages(t, strings.NewReader("\x1b[<0;4;5M\x1b[<0;4;5m\x1b[<64;4;5M\x1b[<35;4;5M\x1b[8;30;100t"))
	if len(msgs) != 5 {
		t.Fatal(msgs)
	}
	click := msgs[0].(tea.MouseMsg)
	if click.X != 3 || click.Y != 4 || click.Button != tea.MouseButtonLeft || click.Action != tea.MouseActionPress || click.Type != tea.MouseLeft {
		t.Fatal(click)
	}
	if release := msgs[1].(tea.MouseMsg); release.Action != tea.MouseActionRelease || release.Type != tea.MouseRelease {
		t.Fatal(release)
	}
	if wheel := msgs[2].(tea.MouseMsg); wheel.Button != tea.MouseButtonWheelUp {
		t.Fatal(wheel)
	}
	if hover := msgs[3].(tea.MouseMsg); hover.X != 3 || hover.Y != 4 || hover.Button != tea.MouseButtonNone || hover.Action != tea.MouseActionMotion {
		t.Fatal(hover)
	}
	if resize := msgs[4].(tea.WindowSizeMsg); resize.Width != 100 || resize.Height != 30 {
		t.Fatal(resize)
	}
}
