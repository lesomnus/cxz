package tui

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestScrollTrackWidthAndPositions(t *testing.T) {
	m := conversationModel()
	m.view.SetContent(strings.Repeat("line\n", 100))
	for _, width := range []int{1, 2, 20, 80, 120} {
		m.width = width
		end := m.view.TotalLineCount() - m.view.Height
		for _, offset := range []int{0, end / 2, end} {
			m.view.SetYOffset(offset)
			bar := ansi.Strip(m.scrollTrack())
			if ansi.StringWidth(bar) != width || strings.Count(bar, "◆︎") != 1 || strings.Contains(bar, " ") {
				t.Fatalf("invalid track: %q", bar)
			}
			if offset == 0 && !strings.HasPrefix(bar, "◆︎") || offset == end && !strings.HasSuffix(bar, "◆︎") {
				t.Fatal("wrong endpoint", bar)
			}
		}
	}
}

func TestInsetRulesHaveSymmetricMargins(t *testing.T) {
	for _, width := range []int{1, 4, 5, 20, 80} {
		line := ansi.Strip(insetRule(width, muted))
		if ansi.StringWidth(line) != width {
			t.Fatal("rule width", line)
		}
		if width > 4 && (!strings.HasPrefix(line, "  ─") || !strings.HasSuffix(line, "─  ")) {
			t.Fatal("rule margins", line)
		}
	}
}

func TestModalDimsComposerWithoutLosingFocus(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := conversationModel()
	m.input.SetValue("draft")
	m.input.Focus()
	m.cursorOutput = &cursorWriter{out: &bytes.Buffer{}}
	m.anchorCursor()
	if !m.cursorOutput.enabled {
		t.Fatal("missing initial cursor")
	}
	top := m.height - m.input.Height() - 3
	active := strings.Split(m.sessionScreen(), "\n")[top]
	for _, kind := range []string{"report", "picker"} {
		if kind == "report" {
			m.openReport("/usage", "report")
		} else {
			m.modelPicker = &modelPicker{kind: "/model"}
		}
		inactive := strings.Split(m.sessionScreen(), "\n")[top]
		m.anchorCursor()
		if inactive == active || m.cursorOutput.enabled || !m.input.Focused() || m.input.Value() != "draft" {
			t.Fatal("modal focus styling/cursor failure", kind)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m.anchorCursor()
		if !m.cursorOutput.enabled || strings.Split(m.sessionScreen(), "\n")[top] != active {
			t.Fatal("focus not restored", kind)
		}
	}
}

func TestScrollTrackUsesSpacerRow(t *testing.T) {
	m := conversationModel()
	m.view.SetContent(strings.Repeat("line\n", 100))
	m.view.SetYOffset(10)
	lines := strings.Split(ansi.Strip(m.sessionScreen()), "\n")
	if len(lines) != m.height || !strings.Contains(lines[m.view.Height], "◆︎") || !strings.Contains(lines[m.view.Height+1], "Ctrl+End latest") {
		t.Fatal("track displaced status/layout")
	}
	if ansi.StringWidth(lines[m.view.Height]) != m.width {
		t.Fatal("track does not fill screen")
	}
}
