package tui

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/erikgeiser/coninput"
	"golang.org/x/sys/windows"
)

type windowsKeyProbe struct {
	ready chan struct{}
	keys  chan tea.KeyMsg
}

func (m *windowsKeyProbe) Init() tea.Cmd { close(m.ready); return nil }
func (m *windowsKeyProbe) View() string  { return "" }
func (m *windowsKeyProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		m.keys <- key
	}
	return m, nil
}

func consoleKeyRecord(key coninput.KeyEventRecord) coninput.InputRecord {
	record := coninput.InputRecord{EventType: coninput.KeyEventType}
	if key.KeyDown {
		binary.LittleEndian.PutUint32(record.Event[0:4], 1)
	}
	binary.LittleEndian.PutUint16(record.Event[4:6], key.RepeatCount)
	binary.LittleEndian.PutUint16(record.Event[6:8], uint16(key.VirtualKeyCode))
	binary.LittleEndian.PutUint16(record.Event[8:10], uint16(key.VirtualScanCode))
	binary.LittleEndian.PutUint16(record.Event[10:12], uint16(key.Char))
	binary.LittleEndian.PutUint32(record.Event[12:16], uint32(key.ControlKeyState))
	return record
}

func TestWindowsConsoleInputProgram(t *testing.T) {
	if os.Getenv("CXZ_TEST_NATIVE_CONSOLE") != "1" {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestWindowsConsoleInputProgram$", "-test.v")
		cmd.Env = append(os.Environ(), "CXZ_TEST_NATIVE_CONSOLE=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("native console probe: %v\n%s", err, out)
		}
		return
	}
	// Detach only this child process, then allocate an isolated console. Neither
	// the test runner's console nor the user's terminal input buffer is changed.
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	free := kernel.NewProc("FreeConsole")
	_, _, _ = free.Call()
	if result, _, err := kernel.NewProc("AllocConsole").Call(); result == 0 {
		t.Fatal("AllocConsole", err)
	}
	defer free.Call()
	file, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	handle := windows.Handle(file.Fd())
	var before uint32
	if err := windows.GetConsoleMode(handle, &before); err != nil {
		t.Fatal(err)
	}
	write := kernel.NewProc("WriteConsoleInputW")
	for run := 0; run < 2; run++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		m := &windowsKeyProbe{ready: make(chan struct{}), keys: make(chan tea.KeyMsg, 16)}
		p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(keyboardInput(file)), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
		done := make(chan error, 1)
		go func() { _, err := runKeyboardProgram(ctx, p, file, io.Discard, nil); done <- err }()
		select {
		case <-m.ready:
		case <-ctx.Done():
			cancel()
			t.Fatal("console startup timed out")
		}
		var during uint32
		if err := windows.GetConsoleMode(handle, &during); err != nil || during&windows.ENABLE_VIRTUAL_TERMINAL_INPUT == 0 {
			cancel()
			t.Fatal("bracketed paste would be stripped", during, err)
		}
		for _, tc := range []struct {
			key  coninput.KeyEventRecord
			want string
		}{
			{coninput.KeyEventRecord{KeyDown: true, RepeatCount: 1, VirtualKeyCode: coninput.VK_RETURN, Char: '\r'}, "enter"},
			{coninput.KeyEventRecord{KeyDown: true, RepeatCount: 1, VirtualKeyCode: coninput.VK_RETURN, Char: '\n', ControlKeyState: coninput.LEFT_CTRL_PRESSED}, "ctrl+s"},
			{coninput.KeyEventRecord{KeyDown: true, RepeatCount: 1, VirtualKeyCode: coninput.VK_RETURN, Char: '\n', ControlKeyState: coninput.RIGHT_CTRL_PRESSED | coninput.ENHANCED_KEY}, "ctrl+s"},
			{coninput.KeyEventRecord{KeyDown: true, RepeatCount: 1, VirtualKeyCode: coninput.VK_LEFT, ControlKeyState: coninput.LEFT_CTRL_PRESSED}, "ctrl+left"},
			{coninput.KeyEventRecord{KeyDown: true, RepeatCount: 1, Char: '한'}, "한"},
		} {
			record := consoleKeyRecord(tc.key)
			var written uint32
			if result, _, err := write.Call(uintptr(handle), uintptr(unsafe.Pointer(&record)), 1, uintptr(unsafe.Pointer(&written))); result == 0 || written != 1 {
				cancel()
				t.Fatal("WriteConsoleInput", err)
			}
			select {
			case key := <-m.keys:
				if key.String() != tc.want {
					cancel()
					t.Fatalf("want %s, got %s", tc.want, key.String())
				}
			case <-ctx.Done():
				cancel()
				t.Fatal("console key not delivered", tc.want)
			}
		}
		// Emulate ConHost's VT-input records for a paste larger than both the
		// console reader's 64-record batch and the decoder's 4096-byte read.
		body := strings.Repeat("한글🙂 long paste\r\n", 512)
		wire := "\x1b[200~" + body + "\x1b[201~" + win32Key(13, '\n', 8, 1, 1)
		var records []coninput.InputRecord
		for _, char := range utf16.Encode([]rune(wire)) {
			records = append(records, consoleKeyRecord(coninput.KeyEventRecord{KeyDown: true, RepeatCount: 1, Char: rune(char)}))
		}
		var written uint32
		if result, _, err := write.Call(uintptr(handle), uintptr(unsafe.Pointer(&records[0])), uintptr(len(records)), uintptr(unsafe.Pointer(&written))); result == 0 || int(written) != len(records) {
			cancel()
			t.Fatal("WriteConsoleInput paste", written, err)
		}
		for i := range 2 {
			select {
			case key := <-m.keys:
				if i == 0 && (!key.Paste || string(key.Runes) != body) {
					cancel()
					t.Fatal("long console paste was split or changed")
				}
				if i == 1 && (key.Paste || key.String() != "ctrl+s") {
					cancel()
					t.Fatal("Ctrl+Enter lost after console paste", key.String())
				}
			case <-ctx.Done():
				cancel()
				t.Fatal("long console paste not delivered")
			}
		}
		p.Quit()
		select {
		case err := <-done:
			if err != nil {
				cancel()
				t.Fatal(err)
			}
		case <-ctx.Done():
			cancel()
			t.Fatal("input reader did not stop")
		}
		cancel()
		var after uint32
		if err := windows.GetConsoleMode(handle, &after); err != nil || after != before {
			t.Fatal("console mode not restored", before, after, err)
		}
	}
}

func TestWindowsConsoleRecordSerialization(t *testing.T) {
	r := &windowsConsoleInput{}
	for _, char := range "\x1b[200~" {
		r.encode(coninput.KeyEventRecord{KeyDown: true, RepeatCount: 1, Char: char})
	}
	r.encode(coninput.KeyEventRecord{KeyDown: true, RepeatCount: 1, VirtualKeyCode: coninput.VK_RETURN, Char: '\n', ControlKeyState: coninput.LEFT_CTRL_PRESSED})
	for _, char := range "\x1b[201~" {
		r.encode(coninput.KeyEventRecord{KeyDown: true, RepeatCount: 1, Char: char})
	}
	msgs := decodedTerminalMessages(t, strings.NewReader(r.buffer.String()))
	if len(msgs) != 1 {
		t.Fatal(msgs)
	}
	if key := msgs[0].(tea.KeyMsg); !key.Paste || string(key.Runes) != "\n" {
		t.Fatal("pasted Enter became a submit", key)
	}
	r.buffer.Reset()
	r.encode(coninput.KeyEventRecord{KeyDown: true, RepeatCount: 1, Char: 0xd83d})
	r.encode(coninput.KeyEventRecord{KeyDown: true, RepeatCount: 1, Char: 0xde42})
	msgs = decodedTerminalMessages(t, strings.NewReader(r.buffer.String()))
	if len(msgs) != 1 || string(msgs[0].(tea.KeyMsg).Runes) != "🙂" {
		t.Fatal(msgs)
	}
}
