package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestFencedCodeBlockBackgroundAndPadding(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	text := markdownView("```go\nx := 1\n\n世界\n```", 20)
	rows := strings.Split(text, "\n")
	if len(rows) != 5 {
		t.Fatalf("unexpected code rows: %q", text)
	}
	for _, row := range rows {
		plain := ansi.Strip(row)
		if ansi.StringWidth(row) != 20 || !strings.HasPrefix(plain, " ") || !strings.Contains(row, "\x1b[48;2;43;43;43m") {
			t.Fatalf("unfilled code row: %q", row)
		}
	}
}
