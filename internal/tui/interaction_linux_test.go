//go:build linux

package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/sys/unix"
)

type keyProbe struct {
	ready chan struct{}
	keys  chan tea.KeyMsg
}

func (m *keyProbe) Init() tea.Cmd { close(m.ready); return nil }
func (m *keyProbe) View() string  { return "" }
func (m *keyProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		m.keys <- key
	}
	return m, nil
}

func TestTerminalCtrlEnterEncoding(t *testing.T) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("PTY unavailable: %v", err)
	}
	defer master.Close()
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := &keyProbe{ready: make(chan struct{}), keys: make(chan tea.KeyMsg, 16)}
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(keyboardInput(slave)), tea.WithOutput(io.Discard))
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	defer func() {
		p.Quit()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-m.ready:
	case <-ctx.Done():
		t.Fatal("TUI startup timeout")
	}
	for _, tc := range []struct {
		wire  string
		want  tea.KeyType
		paste bool
	}{
		{"\r", tea.KeyEnter, false},
		{"\x1b", tea.KeyEsc, false},
		{"\x1b[13;5u", tea.KeyCtrlS, false},
		{"\x1b[?1u\x1b[115;5u", tea.KeyCtrlS, false},
		{"\x1b[99;5u", tea.KeyCtrlC, false},
		{"\x1b[27u", tea.KeyEsc, false},
		{"\x1b[9;2u", tea.KeyShiftTab, false},
		{"\x1b[57414;5u", tea.KeyCtrlS, false},
		{"\x1b[57419u", tea.KeyUp, false},
		{"\x1b[27;5;13~", tea.KeyCtrlS, false},
		{"\x13", tea.KeyCtrlS, false},
		{"\x1b[Z", tea.KeyShiftTab, false},
		{"\x1b[200~paste\n\x1b[13;5u\x1b[201~", tea.KeyRunes, true},
		{"\x1b[200~" + strings.Repeat("한글🙂", 400) + "\x1b[201~", tea.KeyRunes, true},
	} {
		if _, err := master.Write([]byte(tc.wire)); err != nil {
			t.Fatal(err)
		}
		select {
		case key := <-m.keys:
			if key.Type != tc.want || key.Paste != tc.paste {
				t.Fatalf("%q: got %+v", tc.wire, key)
			}
			if tc.paste && string(key.Runes) != strings.TrimSuffix(strings.TrimPrefix(tc.wire, "\x1b[200~"), "\x1b[201~") {
				t.Fatal("paste contents changed")
			}
		case <-ctx.Done():
			t.Fatalf("key %q not decoded", tc.wire)
		}
	}
}

func TestKeyboardReaderSplitBoundaries(t *testing.T) {
	for _, tc := range [][2]string{
		{"한글🙂\x1b[13;5u", "한글🙂\x13"},
		{"\x1b[?1u\x1b[13;69u\x1b[27u\x1b[115;5u", "\x13\x1b\x13"},
		{"\x1b[27;5;13~\x1b[Z\x1b[15~\x1b[<64;123;456M", "\x13\x1b[Z\x1b[15~\x1b[<64;123;456M"},
		{"\x1b[200~한글\x1b[13;5u\x1b[201~\x1b[13;5u", "\x1b[200~한글\x1b[13;5u\x1b[201~\x13"},
	} {
		for split := 1; split < len(tc[0]); split++ {
			r := &keyboardReader{}
			first, pending := r.translate([]byte(tc[0][:split]), false)
			second, pending := r.translate(append(bytes.Clone(pending), []byte(tc[0][split:])...), false)
			last, _ := r.translate(pending, true)
			got := string(append(append(first, second...), last...))
			if got != tc[1] {
				t.Fatalf("split %d: %q != %q", split, got, tc[1])
			}
		}
	}
}

func TestKittyModeReply(t *testing.T) {
	r := &keyboardReader{}
	out, pending := r.translate([]byte("\x1b[?1u"), true)
	if len(out) != 0 || len(pending) != 0 || !r.kittyConfirmed || r.kittyFlags != 1 {
		t.Fatalf("reply not consumed/recorded: %+v %q", r, out)
	}
}
