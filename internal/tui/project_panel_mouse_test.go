package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/vt"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

func TestPanelRowHighlightsPreserveTextStyle(t *testing.T) {
	profile := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(profile)
	for _, colors := range []termenv.Profile{termenv.TrueColor, termenv.ANSI256} {
		lipgloss.SetColorProfile(colors)
		for _, width := range []int{69, 80, 200} {
			for _, index := range []int{0, 1} { // Project label and provider-colored session.
				m := panelModel()
				m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
				m.projectView = true
				m.panelIndex = index
				panelWidth, y := m.panelScreenWidth(), 4+index
				capture := func() *vt.Emulator {
					terminal := vt.NewEmulator(panelWidth, m.height)
					terminal.WriteString(strings.ReplaceAll(m.panelScreen(), "\n", "\r\n"))
					return terminal
				}
				baseline := capture()
				m.Update(tea.MouseMsg{X: panelWidth - 1, Y: y, Action: tea.MouseActionMotion})
				for _, focused := range []bool{false, true} {
					m.panelFocus = focused
					got := capture()
					gray := uint32(0x3a3a)
					if focused {
						gray = 0x4444
					}
					for x := 0; x < panelWidth; x++ {
						cell, original := got.CellAt(x, y), baseline.CellAt(x, y)
						if cell == nil || original == nil || cell.Style.Bg == nil {
							t.Fatalf("unpainted cell at %d,%d", x, y)
						}
						r, g, b, _ := cell.Style.Bg.RGBA()
						if r != gray || g != gray || b != gray {
							t.Fatalf("wrong highlight at %d,%d: %x %x %x", x, y, r, g, b)
						}
						style, originalStyle := cell.Style, original.Style
						style.Bg, originalStyle.Bg = nil, nil
						if !reflect.DeepEqual(style, originalStyle) {
							t.Fatalf("highlight changed text style at %d,%d", x, y)
						}
					}
					got.Close()
				}
				baseline.Close()
			}
		}
	}
}

func TestPanelMouseNavigation(t *testing.T) {
	for _, width := range []int{69, 80, 200} {
		m := panelModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		m.input.SetValue("mouse navigation draft")
		if width < 70 {
			m.focusPanel()
		}
		selected, focused := m.panelIndex, m.input.Focused()
		// The first project is Other project, followed by its maple session.
		x := m.panelScreenWidth() - 1 // Include the padding beyond the label.
		m.Update(tea.MouseMsg{X: x, Y: 4, Action: tea.MouseActionMotion})
		if m.panelHoverY != 4 || m.panelIndex != selected || m.input.Focused() != focused || m.current().Id != "s" {
			t.Fatal("hover changed selection or focus", width)
		}
		m.Update(tea.MouseMsg{X: x, Y: 4, Button: tea.MouseButtonRight, Action: tea.MouseActionPress})
		m.Update(tea.MouseMsg{X: x, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
		if m.panelIndex != selected || m.current().Id != "s" {
			t.Fatal("right click or release activated an item")
		}
		m.Update(tea.MouseMsg{X: x, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if !m.panelFocus || m.input.Focused() || m.panelIndex != 1 || m.current().Id != "s" {
			t.Fatal("project click did not select its first session")
		}
		m.Update(tea.MouseMsg{X: x, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.panelFocus || !m.input.Focused() || m.current().Id != "other-session" || m.project.Id != "other" || m.drafts["s"] != "mouse navigation draft" {
			t.Fatal("session click lost the draft or opened the wrong session")
		}
	}
}

func TestPanelMouseScrolledRowsAndHoverReset(t *testing.T) {
	m := panelModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	var sessions []*api.Session
	for i := 0; i < 40; i++ {
		sessions = append(sessions, &api.Session{Id: fmt.Sprintf("s%d", i), ProjectId: "p", Alias: fmt.Sprintf("session%02d", i)})
	}
	m.updatePanel(listing{projects: []*api.Project{m.project}, sessions: sessions})
	m.focusPanel()
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m.Update(tea.MouseMsg{X: 2, Y: 5, Button: tea.MouseButtonWheelUp})
	if m.panelIndex != 37 {
		t.Fatal("wheel did not move through the project list", m.panelIndex)
	}
	rows := m.panelRows()
	start, _ := m.panelRange(len(rows))
	target := rows[start+1].session.Id
	m.Update(tea.MouseMsg{X: 2, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.current().Id != target {
		t.Fatal("click ignored the scrolled list offset")
	}
	m.Update(tea.MouseMsg{X: 2, Y: 5, Action: tea.MouseActionMotion})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if m.panelHoverY != 0 {
		t.Fatal("keyboard navigation left a stale hover")
	}
	m.Update(tea.MouseMsg{X: 2, Y: 5, Action: tea.MouseActionMotion})
	m.Update(tea.MouseMsg{X: m.panelWidth(), Y: 5, Action: tea.MouseActionMotion})
	if m.panelHoverY != 0 {
		t.Fatal("hover remained outside the panel")
	}
	m.Update(tea.MouseMsg{X: 2, Y: 5, Action: tea.MouseActionMotion})
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	if m.panelHoverY != 0 {
		t.Fatal("resize left a stale hover")
	}
	// A pending deletion must retain its selected target despite mouse input.
	selected := m.panelIndex
	m.deletingID = target
	m.Update(tea.MouseMsg{X: 2, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.panelIndex != selected || m.deletingID != target || m.panelHoverY != 0 {
		t.Fatal("click changed a pending action")
	}
}
