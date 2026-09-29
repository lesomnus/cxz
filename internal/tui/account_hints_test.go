package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/resource"
	"github.com/muesli/termenv"
)

// The accounts screen is a menu like the project panel, so it reads like one:
// the cursor marks where the keyboard is, and a key is coloured inside the word
// it opens instead of sitting in a column of its own.
func TestAccountsViewDrawsAMenuLikeThePanel(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := projectModel()
	m.openAccounts(false)
	m.accounts = []*resource.Account{
		resource.Account_builder{Alias: "first", Agent: "claude", Name: "First"}.Build(),
		resource.Account_builder{Alias: "second", Agent: "codex", Name: "Second"}.Build(),
	}
	m.accountIndex = 1
	screen := m.accountScreen()

	bright := sgr(focus)
	cursor := ""
	for _, row := range strings.Split(screen, "\n") {
		if strings.Contains(ansi.Strip(row), "› second") {
			cursor = row
			break
		}
	}
	if cursor == "" {
		t.Fatal("the accounts list drew no cursor")
	}
	if !strings.Contains(cursor, bright) {
		t.Fatalf("the row under the cursor is not on the focus step: %q", cursor)
	}

	// Rendered by the same hint the panel footer uses, so the two cannot drift.
	folded := hintSpec{key: "n", label: "new account"}.render(false)
	if !strings.Contains(screen, folded) {
		t.Fatalf("a key is not coloured inside its word: %q missing", folded)
	}
	if strings.Contains(ansi.Strip(screen), "n new account") {
		t.Fatal("the key was repeated in front of the word it opens")
	}
}
