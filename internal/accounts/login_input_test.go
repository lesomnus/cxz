package accounts

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestLoginCodeIndicator(t *testing.T) {
	var sent bytes.Buffer
	m := newLoginInput(&sent)
	if !strings.Contains(ansi.Strip(m.View()), "[   ] 000;") {
		t.Fatal(m.View())
	}
	for _, code := range []string{"x", "xx", "xxx", "xxxx", "synthetic-code#synthetic-state", "한글"} {
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(code)})
		n := len([]rune(code))
		want := fmt.Sprintf("[%s%s] %03d; ctrl+x to clear.", strings.Repeat("*", min(3, n)), strings.Repeat(" ", max(0, 3-n)), n)
		if !strings.Contains(ansi.Strip(m.View()), want) {
			t.Fatal("incorrect private indicator")
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if m.input.Value() != "" || !strings.Contains(m.View(), "[   ]") {
		t.Fatal("clear failed")
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("submitted empty code")
	}
	secret := "synthetic-code#synthetic-state"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(secret)})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("missing submit")
	}
	m.Update(cmd())
	if sent.String() != secret+"\n" || m.input.Value() != "" || strings.Contains(m.View(), secret) {
		t.Fatal("submission not private")
	}
	for _, key := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		m := newLoginInput(&sent)
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(secret)})
		m.Update(tea.KeyMsg{Type: key})
		if !m.canceled || m.input.Value() != "" {
			t.Fatal("cancel retained code")
		}
	}
}

func TestLoginWaitingSpinner(t *testing.T) {
	m := newLoginInput(&bytes.Buffer{})
	m.submitted = true
	m.submittedAt = time.Now().Add(-3 * time.Second)
	before := m.View()
	_, cmd := m.Update(loginPulse(time.Now()))
	if cmd == nil || before == m.View() || !strings.Contains(m.View(), "waiting for Claude login") {
		t.Fatal(m.View())
	}
}

func TestClaudeLoginInputProcess(t *testing.T) {
	for _, tc := range []struct {
		name, input, script string
		fail                bool
	}{
		{"submit", "synthetic#state\r", `printf 'Visit https://example.invalid/login\nPaste code > '; IFS= read -r code; test "$code" = 'synthetic#state'`, false},
		{"clear", "discarded\x18synthetic#state\r", `IFS= read -r code; test "$code" = 'synthetic#state'`, false},
		{"cancel", "discarded\x03", `IFS= read -r code`, true},
		{"vendor-failure", "", `exit 1`, true},
		{"browser-completed", "", `printf 'Login successful.\n'`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var output bytes.Buffer
			err := runClaudeLogin(ctx, exec.CommandContext(ctx, "sh", "-c", tc.script), strings.NewReader(tc.input), &output)
			if (err != nil) != tc.fail {
				t.Fatal(err)
			}
			if ctx.Err() != nil {
				t.Fatal("login UI hung")
			}
			for _, secret := range []string{"synthetic#state", "discarded"} {
				if strings.Contains(output.String(), secret) {
					t.Fatal("secret rendered")
				}
			}
		})
	}
}

func TestLoginCounterDimmingAndBlink(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	var sink bytes.Buffer
	m := newLoginInput(&sink)
	m.input.SetValue("x")
	m.input.Cursor.Blink = false
	visible := m.View()
	m.input.Cursor.Blink = true
	hidden := m.View()
	if visible == hidden || !strings.Contains(visible, "\x1b[7m") || strings.Contains(hidden, "\x1b[7m") {
		t.Fatal("cursor does not blink")
	}
	if !strings.Contains(visible, "\x1b[38;2;48;52;59m00\x1b[0m1") {
		t.Fatal("padding zeros not dimmed", visible)
	}
	if strings.Contains(ansi.Strip(visible), "x001") || strings.Contains(ansi.Strip(visible), "[x") {
		t.Fatal("code leaked")
	}
}
