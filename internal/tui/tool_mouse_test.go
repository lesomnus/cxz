package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/muesli/termenv"
	"strings"
	"testing"
	"time"
)

func TestToolDoubleClickAndHover(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := toolViewModel()
	m.view.GotoTop()
	y := m.toolTargets()[0].row - m.view.YOffset
	click := tea.MouseMsg{X: m.contentOffset() + 6, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	before := m.view.View()
	base := m.conversationView()
	m.Update(tea.MouseMsg{X: click.X, Y: y, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion})
	hover := m.conversationView()
	if m.toolHover == nil || !strings.Contains(hover, "\x1b[48;5;236m") || ansi.Strip(hover) != ansi.Strip(base) {
		t.Fatalf("hover changed geometry or missing highlight: target=%+v highlight=%v base=%q hover=%q", m.toolHover, strings.Contains(hover, "\x1b[48;5;236m"), ansi.Strip(base), ansi.Strip(hover))
	}
	m.Update(click)
	if m.filePreview != nil || m.textSelection == nil {
		t.Fatal("single click must select text, not open details")
	}
	if m.view.View() != before {
		t.Fatal("single click changed transcript")
	}
	m.Update(tea.MouseMsg{X: click.X, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	m.Update(click)
	if m.filePreview == nil || m.filePreview.tabs == nil || m.textSelection != nil {
		t.Fatal("double click did not open details")
	}
}
func TestToolDoubleClickRejectsSlowDifferentAndDraggedClicks(t *testing.T) {
	for _, scenario := range []string{"slow", "different", "drag", "scroll"} {
		m := toolViewModel()
		m.view.GotoTop()
		targets := m.toolTargets()
		y := targets[0].row
		click := tea.MouseMsg{X: 6, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
		now := time.Unix(100, 0)
		m.toolMouse(click, now)
		switch scenario {
		case "slow":
			now = now.Add(time.Second)
		case "different":
			click.Y = targets[1].row
		case "drag":
			m.beginSelection(click)
			m.Update(tea.MouseMsg{X: m.contentOffset() + 10, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
		case "scroll":
			m.Update(tea.MouseMsg{X: m.contentOffset() + 6, Y: y, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		}
		if m.toolMouse(click, now.Add(time.Millisecond)) || m.filePreview != nil {
			t.Fatal("unintended preview", scenario)
		}
	}
}
func TestToolHoverClearsOutsideConversation(t *testing.T) {
	m := toolViewModel()
	m.view.GotoTop()
	y := m.toolTargets()[0].row
	m.Update(tea.MouseMsg{X: m.contentOffset() + 4, Y: y, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion})
	if m.toolHover == nil {
		t.Fatal("no hover")
	}
	m.Update(tea.MouseMsg{X: m.contentOffset() + 4, Y: m.height - 1, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion})
	if m.toolHover != nil {
		t.Fatal("hover remained after leaving conversation")
	}
}
func TestProviderHeaderBackgroundCoversSpaces(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	header := strings.Split(renderResponse("codex", "hello", 40).body, "\n")[0]
	if ansi.Strip(header) != " • CODEX " {
		t.Fatal(header)
	}
	terminal := vt.NewEmulator(40, 3)
	defer terminal.Close()
	terminal.Write([]byte(header))
	for x := 0; x < 9; x++ {
		cell := terminal.CellAt(x, 0)
		if cell == nil || cell.Style.Bg == nil {
			t.Fatal("background hole at", x)
		}
		r, g, b, _ := cell.Style.Bg.RGBA()
		if r != 0 || g != 0 || b != 0 {
			t.Fatal("not black at", x)
		}
	}
	if cell := terminal.CellAt(9, 0); cell != nil && cell.Style.Bg != nil {
		t.Fatal("background extends past right padding")
	}
	if got := ansi.Strip(strings.Split(renderResponse("claude", "hello", 40).body, "\n")[0]); got != "• CLAUDE" {
		t.Fatal("non-background provider gained padding", got)
	}
}
