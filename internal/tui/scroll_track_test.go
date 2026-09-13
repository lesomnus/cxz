package tui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

func TestScrollTrackStableAcrossHistoryPages(t *testing.T) {
	m := conversationModel()
	m.width = 100
	page := func(start, end uint64) []*api.Event {
		var events []*api.Event
		for seq := start + 1; seq <= end; seq++ {
			events = append(events, &api.Event{Seq: seq, Kind: "assistant", Text: fmt.Sprintf("response %d\nsecond line", seq)})
		}
		return events
	}
	m.applyHistoryPage(historyPage{id: "s", start: 256, initial: true, events: page(256, 384)})
	m.view.SetYOffset(5)
	before := ansi.Strip(m.scrollTrack())
	for _, start := range []uint64{128, 0} {
		m.applyHistoryPage(historyPage{id: "s", start: start, end: start + 128, events: page(start, start+128)})
		if got := ansi.Strip(m.scrollTrack()); got != before {
			t.Fatalf("prepend moved handle:\n%s\n%s", before, got)
		}
	}
	previous := strings.Index(ansi.Strip(m.scrollTrack()), "◆︎")
	for m.view.YOffset > 0 {
		m.view.SetYOffset(max(0, m.view.YOffset-7))
		next := strings.Index(ansi.Strip(m.scrollTrack()), "◆︎")
		if next > previous {
			t.Fatal("scrolling up moved handle right")
		}
		previous = next
	}
	if previous != 0 {
		t.Fatal("oldest endpoint")
	}
	m.view.GotoBottom()
	if !strings.HasSuffix(ansi.Strip(m.scrollTrack()), "◆︎") {
		t.Fatal("latest endpoint")
	}
}

func TestScrollTrackPastIsBrighter(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := conversationModel()
	m.width = 21
	m.view.SetContent(strings.Repeat("line\n", 100))
	m.view.SetYOffset((m.view.TotalLineCount() - m.view.Height) / 2)
	bar := m.scrollTrack()
	if !strings.HasPrefix(bar, muted.Render(strings.Repeat("─", 10))) || !strings.HasSuffix(bar, zeroStyle.Render(strings.Repeat("─", 10))) {
		t.Fatalf("missing distinct track colors: %q", bar)
	}
}

func TestScrollTrackNearBottomUsesScrollableRange(t *testing.T) {
	m := conversationModel()
	m.width = 134
	m.view.Height = 64
	m.view.SetContent(strings.TrimSuffix(strings.Repeat("line\n", 574), "\n"))
	m.current().LastSeq = 3000 // includes invisible status/usage events at the tail
	m.historyPositions = make([]float64, 574)
	for i := range m.historyPositions {
		m.historyPositions[i] = float64(i + 1)
	}
	m.view.GotoBottom()
	m.view.SetYOffset(m.view.YOffset - 3)
	bar := ansi.Strip(m.scrollTrack())
	left, _, _ := strings.Cut(bar, "◆︎")
	if gap := m.width - 1 - ansi.StringWidth(left); gap > 2 {
		t.Fatalf("three lines left a %d-column gap: %s", gap, bar)
	}
	// Resizing changes how far the viewport can scroll, not the journal tail.
	m.view.Height = 100
	m.view.GotoBottom()
	m.view.SetYOffset(m.view.YOffset - 3)
	left, _, _ = strings.Cut(ansi.Strip(m.scrollTrack()), "◆︎")
	if m.width-1-ansi.StringWidth(left) > 2 {
		t.Fatal("resize retained old scroll endpoint")
	}
}

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
