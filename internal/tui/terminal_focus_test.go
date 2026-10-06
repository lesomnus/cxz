package tui

import (
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/lesomnus/bed"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

// spinnerFrame finds the spinner glyph in a rendered row. Tests assert that a
// spinner is present and advances, never which frame it is on: the frame is a
// function of when the work started, which is the point of the phase.
func spinnerFrame(s string) string {
	for _, r := range s {
		if strings.ContainsRune("⣟⣯⣷⣾⣽⣻⢿⡿", r) {
			return string(r)
		}
	}
	return ""
}

// The bright green means "the keyboard is here". A window sitting in the
// background does not hold the keyboard, so it has to give the step back; what
// is running stays bright, because work continues in a window you are not
// looking at.
func TestBlurredTerminalGivesUpTheFocusStep(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old); keyboardStep(false) })
	m := &model{input: newComposer()}
	bright, quiet := sgr(lipgloss.NewStyle().Foreground(lipgloss.Color(focusGreen))), sgr(lipgloss.NewStyle().Foreground(lipgloss.Color(accentGreen)))

	// A render sets the step from the model that draws, which is also what keeps
	// one blurred window from leaving the next one dim.
	keyboardStep(m.blurred)
	if got := sgr(focus); got != bright {
		t.Fatal("focused window is not on the bright step:", got)
	}
	m.Update(tea.BlurMsg{})
	if !m.blurred {
		t.Fatal("blur not recorded")
	}
	for name, got := range map[string]string{"focus": sgr(focus), "cursor": sgr(inputCursorStyle), "live cursor": sgr(m.input.Cursor.Style)} {
		if got != quiet {
			t.Fatal(name, "still claims the keyboard:", got)
		}
	}
	if got := sgr(running); got != bright {
		t.Fatal("a running spinner dimmed with the window:", got)
	}
	m.Update(tea.FocusMsg{})
	if m.blurred || sgr(focus) != bright || sgr(m.input.Cursor.Style) != bright {
		t.Fatal("focus did not take the bright step back")
	}
}

func TestComposerCursorUsesNativeTimer(t *testing.T) {
	m := &model{composerBlink: true, input: newComposer(), cursorOutput: &cursorWriter{out: io.Discard}}
	m.input.Cursor.SetMode(cursor.CursorBlink)
	m.input.Cursor.BlinkSpeed = time.Millisecond
	cmd := m.input.Focus()
	// Rendering and unrelated animation ticks must leave the timer alive.
	m.input.View()
	for range 10 {
		m.Update(pulseTick{})
	}
	msg := cmd()
	if _, ok := msg.(cursor.BlinkMsg); !ok {
		t.Fatalf("timer canceled: %T", msg)
	}
	_, cmd = m.Update(msg)
	if !m.input.Cursor.Blink || cmd == nil {
		t.Fatal("native cursor did not blink and reschedule")
	}
	// An early-return view route must not swallow the next cursor tick.
	m.projectView = true
	_, cmd = m.Update(cmd())
	if m.input.Cursor.Blink || cmd == nil {
		t.Fatal("view routing swallowed cursor tick")
	}
	m.Update(tea.BlurMsg{})
	t.Cleanup(func() { keyboardStep(false) })
	if m.input.Cursor.Mode() != cursor.CursorStatic {
		t.Fatal("background cursor still blinking")
	}
	m.Update(tea.FocusMsg{})
	if m.input.Cursor.Mode() != cursor.CursorBlink || m.input.Cursor.Blink {
		t.Fatal("focus did not restore native blinking")
	}
}

func TestComposerFocusCommandSurvivesEarlyReturn(t *testing.T) {
	m := &model{composerBlink: true, input: newComposer(), cursorOutput: &cursorWriter{out: io.Discard}}
	m.input.Cursor.SetMode(cursor.CursorBlink)
	m.input.Cursor.BlinkSpeed = time.Millisecond
	m.focusComposer()
	_, cmd := m.Update(bed.CopyMsg(""))
	if cmd == nil {
		t.Fatal("queued focus command lost")
	}
	if _, ok := cmd().(cursor.BlinkMsg); !ok {
		t.Fatal("focus command did not produce a blink")
	}
	if m.composerFocusCmd != nil {
		t.Fatal("focus command not drained")
	}
}

// Two spinners stepping in lockstep read as one animation rather than two
// things working, so each takes its phase from its own work.
func TestSpinnersKeepIndependentPhase(t *testing.T) {
	m := &model{pulse: 3}
	first := &api.Session{Id: "a", State: "working", RunId: "r", CreatedAt: 1_700_000_000_000}
	second := &api.Session{Id: "b", State: "working", RunId: "r", CreatedAt: 1_700_000_000_700}
	a, b := m.sessionIndicator(first), m.sessionIndicator(second)
	if a == "" || b == "" {
		t.Fatal("no spinner rendered", a, b)
	}
	if a == b {
		t.Fatal("sessions started 700ms apart share a phase:", a)
	}
	// The phase is a property of the work, not of the frame: it has to hold
	// still while the pulse advances, or the spinner stutters.
	m.pulse++
	next := m.sessionIndicator(first)
	if spinnerFrame(next) == "" || next == a {
		t.Fatal("spinner did not advance with the pulse")
	}
	if spinnerPhase(0) != 0 {
		t.Fatal("unknown start time must not shift the phase")
	}
	if spinnerKeyPhase("x") == spinnerKeyPhase("y") {
		t.Fatal("keyed phases collide")
	}
}
