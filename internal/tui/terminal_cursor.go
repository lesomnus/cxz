package tui

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/muesli/termenv"
)

// Bubble Tea v1's alternate-screen renderer homes before each frame and parks
// the physical cursor on the last row afterward. IME preedit/candidate windows
// follow that physical cursor, not the textarea's simulated cursor. Re-anchor
// after writes only while our alternate screen is active. tea.Exec/OAuth writes
// are untouched after the renderer leaves the alternate screen.
type cursorWriter struct {
	mu                 sync.Mutex
	out                io.Writer
	x, y               int
	enabled, alternate bool
	keyboard           bool
}

var _ term.File = (*cursorWriter)(nil)

// term.File also requires Read and Close, even for output-only terminal use.
func (w *cursorWriter) Read(p []byte) (int, error) {
	if r, ok := w.out.(io.Reader); ok {
		return r.Read(p)
	}
	return 0, errors.ErrUnsupported
}

func (w *cursorWriter) Close() error {
	if c, ok := w.out.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// Preserve terminal detection, initial sizing and SIGWINCH handling when Bubble
// Tea receives this wrapper instead of stdout. Non-file writers have no valid fd.
func (w *cursorWriter) Fd() uintptr {
	if f, ok := w.out.(term.File); ok {
		return f.Fd()
	}
	return ^uintptr(0)
}

func (w *cursorWriter) position(x, y int, enabled bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.x, w.y, w.enabled = x, y, enabled
}
func (w *cursorWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	buf := p
	// Keyboard stacks belong to each screen: push AFTER entering, pop BEFORE
	// leaving. tea.Exec also leaves/re-enters, keeping OAuth subprocesses legacy.
	if w.keyboard {
		buf = bytes.ReplaceAll(buf, []byte("\x1b[?1049h"), []byte("\x1b[?1049h\x1b[>1u\x1b[?u"))
		buf = bytes.ReplaceAll(buf, []byte("\x1b[?1049l"), []byte("\x1b[<u\x1b[?1049l"))
	}
	if bytes.Contains(p, []byte("\x1b[?1049h")) {
		w.alternate = true
	}
	if bytes.Contains(p, []byte("\x1b[?1049l")) {
		w.alternate = false
	}
	if w.alternate && w.enabled {
		buf = append(append([]byte{}, buf...), []byte(ansi.CursorPosition(w.x+1, w.y+1))...)
	}
	n, err := w.out.Write(buf)
	if n < len(buf) && err == nil {
		err = io.ErrShortWrite
	}
	return min(n, len(p)), err
}

var sgrPattern = regexp.MustCompile("\x1b\\[([0-9;]*)m")
var cursorProbeStyle = func() lipgloss.Style {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	return r.NewStyle()
}()

// Inspect a forced-visible copy of the widget's own cursor. This incorporates
// soft wrapping, Unicode cell widths and its private viewport scroll offset.
func widgetCursor(view string) (x, y int, ok bool) {
	for row, line := range strings.Split(view, "\n") {
		for _, match := range sgrPattern.FindAllStringSubmatchIndex(line, -1) {
			for _, code := range strings.Split(line[match[2]:match[3]], ";") {
				if code == "7" {
					return ansi.StringWidth(line[:match[0]]), row, true
				}
			}
		}
	}
	return 0, 0, false
}

func (m *model) anchorCursor() {
	if m.cursorOutput == nil {
		return
	}
	x, y, ok := 0, 0, false
	if m.width >= 40 && m.height >= 14 {
		switch {
		case m.pasteDialog != nil:
		case m.workflow != nil:
			// The workflow owns its masked input; never anchor to the hidden form.
		case m.questionDialog != nil:
			q := m.questionDialog.questions[m.questionDialog.page]
			if !q.Other || m.questionDialog.row != len(q.Options) || m.questionDialog.sending {
				break
			}
			copy := *m
			d := *m.questionDialog
			d.other = append([]textinput.Model(nil), d.other...)
			for i := range d.other {
				d.other[i].Cursor.Style = cursorProbeStyle
			}
			copy.questionDialog = &d
			copy.pulse = 0
			x, y, ok = widgetCursor(copy.questionOverlay(copy.conversationView()))
		case m.accountView:
			copy := *m
			copy.accountAlias.Cursor.Blink = false
			copy.accountName.Cursor.Blink = false
			copy.accountSearch.Cursor.Blink = false
			copy.accountAlias.Cursor.Style = cursorProbeStyle
			copy.accountName.Cursor.Style = cursorProbeStyle
			copy.accountSearch.Cursor.Style = cursorProbeStyle
			x, y, ok = widgetCursor(copy.accountScreen())
		case !m.projectView && !m.focusList && !m.focusApproval && m.report == nil && m.modelPicker == nil && m.restartConfirm == nil && m.questionDialog == nil:
			copy := m.input
			copy.Cursor.Blink = false
			copy.Cursor.Style = cursorProbeStyle
			x, y, ok = widgetCursor(copy.View())
			x++
			y += m.height - m.input.Height() - 2
		}
	}
	m.cursorOutput.position(x, y, ok)
}
