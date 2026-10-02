package tui

import (
	"strings"
	"testing"

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

// The widget's own blink is a chain that dies as soon as one tick is consumed
// by another branch of Update, and nothing restarts it until the input is
// refocused. The pulse cannot stop, so the phase is read from it.
func TestComposerCursorBlinksOnThePulse(t *testing.T) {
	m := &model{input: newComposer()}
	phase := func() bool { return m.input.Cursor.Blink }
	pulse := func(n int) {
		for range n {
			m.Update(pulseTick{})
		}
	}
	pulse(5)
	if !phase() {
		t.Fatal("cursor never blinked off")
	}
	pulse(5)
	if phase() {
		t.Fatal("cursor never came back on")
	}
	// Typing restarts the phase: a cursor blinked off as a character lands reads
	// as lag.
	pulse(5)
	if !phase() {
		t.Fatal("phase did not advance")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	pulse(1)
	if phase() {
		t.Fatal("a keystroke did not restore the cursor")
	}
	// A background window holds it still; there is nothing to type into.
	m.Update(tea.BlurMsg{})
	t.Cleanup(func() { keyboardStep(false) })
	pulse(20)
	if phase() {
		t.Fatal("cursor blinked off while the window was blurred")
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
