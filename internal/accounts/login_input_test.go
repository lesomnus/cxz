package accounts

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLoginCodeIndicator(t *testing.T) {
	var sent bytes.Buffer
	m := newLoginInput(&sent)
	if !strings.Contains(m.View(), "[     ]") {
		t.Fatal(m.View())
	}
	for _, code := range []string{"x", "synthetic-code#synthetic-state", "한글"} {
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(code)})
		if m.View() != "\n[ *** ] inserted; ctrl+x to clear.\nEnter to submit; Esc/Ctrl-C to cancel.\n" {
			t.Fatal("incorrect private indicator")
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if m.input.Value() != "" || !strings.Contains(m.View(), "[     ]") {
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
