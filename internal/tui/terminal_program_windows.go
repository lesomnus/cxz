package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"
	"unicode/utf16"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/erikgeiser/coninput"
	"golang.org/x/sys/windows"
)

// Own console input while the program runs, keeping the native modifier bits.
// Feeding bytes back through Bubble Tea v1 would lose Unicode to its ACP reader,
// so decode using Ultraviolet and deliver typed messages directly to the model.
func runKeyboardProgram(ctx context.Context, p *tea.Program, in io.Reader, out io.Writer, recorder *debugRecorder) (tea.Model, error) {
	file, ok := in.(*os.File)
	if !ok {
		return p.Run()
	}
	handle := windows.Handle(file.Fd())
	var original uint32
	if err := windows.GetConsoleMode(handle, &original); err != nil {
		return p.Run()
	}
	// Without VT input, ConHost strips bracketed-paste boundaries and turns a
	// paste into thousands of individual keystrokes. Win32 input mode on the
	// output preserves Ctrl+Enter and other native modifiers alongside VT paste.
	mode := uint32(windows.ENABLE_WINDOW_INPUT | windows.ENABLE_MOUSE_INPUT | windows.ENABLE_EXTENDED_FLAGS | windows.ENABLE_VIRTUAL_TERMINAL_INPUT)
	if err := windows.SetConsoleMode(handle, mode); err != nil {
		return nil, fmt.Errorf("prepare keyboard: %w", err)
	}
	defer windows.SetConsoleMode(handle, original)
	// Keep Bubble Tea's byte reader inert without using nil: RestoreTerminal
	// unconditionally recreates that reader and would otherwise dereference nil.
	tea.WithInput(bytes.NewReader(nil))(p)
	tea.WithOutput(&cursorWriter{out: out, win32Keyboard: true})(p)
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	input := &windowsConsoleInput{ctx: readCtx, handle: handle}
	reader := uv.NewTerminalReader(input, os.Getenv("TERM"))
	events := make(chan uv.Event)
	done := make(chan error, 1)
	forwarded := make(chan struct{})
	go func() {
		err := reader.StreamEvents(readCtx, events)
		close(events)
		done <- err
		if err != nil {
			p.Kill()
		}
	}()
	go func() {
		defer close(forwarded)
		for event := range events {
			if key, ok := event.(uv.KeyPressEvent); ok && key.Code == uv.KeyEnter && key.Mod&^(uv.ModCapsLock|uv.ModNumLock|uv.ModScrollLock) == uv.ModCtrl {
				recorder.Add(debugEvent{Kind: "terminal_navigation", Type: "windows_ctrl_enter"})
			}
			for _, msg := range terminalEventMessages(event) {
				p.Send(msg)
			}
		}
	}()
	model, err := p.Run()
	cancel()
	inputErr := <-done
	<-forwarded
	if inputErr != nil {
		return model, fmt.Errorf("read console input: %w", inputErr)
	}
	return model, err
}

// Pass the console's raw VT text through as UTF-8. Serialize only actual native
// keys to Win32 sequences so their modifiers survive the incremental decoder.
type windowsConsoleInput struct {
	ctx      context.Context
	handle   windows.Handle
	buffer   bytes.Buffer
	raw      bytes.Buffer
	text     consoleUTF16
	decoder  consoleVTDecoder
	buttons  coninput.ButtonState
	lastRead time.Time
}

func (r *windowsConsoleInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for r.buffer.Len() == 0 {
		if r.ctx.Err() != nil {
			return 0, io.EOF
		}
		// Drain pasted text in batches without waiting for the batch to fill.
		records, err := coninput.PeekNConsoleInputs(r.handle, 1024)
		if err != nil {
			return 0, err
		}
		if len(records) == 0 {
			if len(r.decoder.pending) > 0 && time.Since(r.lastRead) >= uv.DefaultEscTimeout {
				r.decoder.write(&r.buffer, nil, true)
				continue
			}
			select {
			case <-r.ctx.Done():
				return 0, io.EOF
			case <-time.After(10 * time.Millisecond):
				continue
			}
		}
		records, err = coninput.ReadNConsoleInputs(r.handle, uint32(len(records)))
		if err != nil {
			return 0, err
		}
		for _, record := range records {
			r.encode(record.Unwrap())
		}
		r.lastRead = time.Now()
		r.decoder.write(&r.buffer, r.raw.Bytes(), false)
		r.raw.Reset()
	}
	return r.buffer.Read(p)
}

func (r *windowsConsoleInput) encode(event coninput.EventRecord) {
	switch e := event.(type) {
	case coninput.KeyEventRecord:
		if !e.KeyDown {
			return
		}
		vk := e.VirtualKeyCode
		const vkPacket coninput.VirtualKeyCode = 0xe7 // Unicode input / IME packet.
		if utf16.IsSurrogate(e.Char) || vk == vkPacket {
			vk = 0
		}
		// Expand repeats explicitly: raw Unicode (VK=0) is decoded as text.
		for i := uint16(0); i < e.RepeatCount; i++ {
			if vk == 0 {
				r.text.write(&r.raw, e.Char)
				continue
			}
			fmt.Fprintf(&r.raw, "\x1b[%d;%d;%d;1;%d;1_", vk, e.VirtualScanCode, e.Char, e.ControlKeyState)
		}
	case coninput.WindowBufferSizeEventRecord:
		fmt.Fprintf(&r.raw, "\x1b[8;%d;%dt", e.Size.Y, e.Size.X)
	case coninput.MouseEventRecord:
		r.encodeMouse(e)
	case coninput.FocusEventRecord:
		if e.SetFocus {
			r.raw.WriteString("\x1b[I")
		} else {
			r.raw.WriteString("\x1b[O")
		}
	}
}

func (r *windowsConsoleInput) encodeMouse(e coninput.MouseEventRecord) {
	button := ansi.MouseNone
	release, motion := false, e.EventFlags == coninput.MOUSE_MOVED
	buttons := e.ButtonState & 0xffff
	switch e.EventFlags {
	case coninput.MOUSE_WHEELED:
		button = ansi.MouseWheelDown
		if e.WheelDirection > 0 {
			button = ansi.MouseWheelUp
		}
	case coninput.MOUSE_HWHEELED:
		button = ansi.MouseWheelLeft
		if e.WheelDirection > 0 {
			button = ansi.MouseWheelRight
		}
	default:
		changed := r.buttons ^ buttons
		release = changed != 0 && changed&buttons == 0
		active := changed
		if active == 0 {
			active = buttons
		}
		for _, b := range []struct {
			flag   coninput.ButtonState
			button ansi.MouseButton
		}{
			{coninput.FROM_LEFT_1ST_BUTTON_PRESSED, ansi.MouseLeft},
			{coninput.FROM_LEFT_2ND_BUTTON_PRESSED, ansi.MouseMiddle},
			{coninput.RIGHTMOST_BUTTON_PRESSED, ansi.MouseRight},
			{coninput.FROM_LEFT_3RD_BUTTON_PRESSED, ansi.MouseBackward},
			{coninput.FROM_LEFT_4TH_BUTTON_PRESSED, ansi.MouseForward},
		} {
			if active&b.flag != 0 {
				button = b.button
				break
			}
		}
	}
	r.buttons = buttons
	state := e.ControlKeyState
	code := ansi.EncodeMouseButton(button, motion, state&coninput.SHIFT_PRESSED != 0,
		state&(coninput.LEFT_ALT_PRESSED|coninput.RIGHT_ALT_PRESSED) != 0,
		state&(coninput.LEFT_CTRL_PRESSED|coninput.RIGHT_CTRL_PRESSED) != 0)
	r.raw.WriteString(ansi.MouseSgr(code, int(e.MousePositon.X), int(e.MousePositon.Y), release))
}
