package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestRestartModalDefaultCancelAndInputCapture(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyEnter, tea.KeyEsc, tea.KeyCtrlQ} {
		m := conversationModel()
		m.input.SetValue("draft")
		m.restartCommand("/restart")
		before := m.view.View()
		offset := m.view.YOffset
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
		if m.restartConfirm == nil || m.restartConfirm.confirm || m.input.Value() != "draft" {
			t.Fatal("modal did not capture input")
		}
		screen := ansi.Strip(m.sessionScreen())
		if !strings.Contains(screen, "[ Confirm ]") || !strings.Contains(screen, "[ Cancel ]") || !strings.Contains(screen, "╭") {
			t.Fatal(screen)
		}
		if before != m.view.View() || offset != m.view.YOffset {
			t.Fatal("modal changed history")
		}
		_, cmd := m.Update(tea.KeyMsg{Type: key})
		if cmd != nil || m.restartConfirm != nil || m.input.Value() != "draft" {
			t.Fatal("cancel did not restore composer")
		}
	}
}

func TestRestartModalMouseAndNarrowLayout(t *testing.T) {
	for _, size := range [][2]int{{80, 15}, {27, 8}, {20, 6}, {15, 4}} {
		for _, confirm := range []bool{false, true} {
			m := conversationModel()
			m.width = size[0]
			m.view.Height = size[1]
			c := &restartClient{run: "run", state: "idle"}
			m.client = c
			m.restartCommand("/restart")
			lines, buttons := m.restartLayout(m.view.Height)
			if len(buttons) != 2 || len(lines) > m.view.Height-2 {
				t.Fatal("buttons not visible")
			}
			view := m.restartOverlay(strings.Repeat(strings.Repeat(" ", m.width)+"\n", m.view.Height-1) + strings.Repeat(" ", m.width))
			rows := strings.Split(ansi.Strip(view), "\n")
			for _, b := range buttons {
				if !strings.Contains(rows[b.y], map[bool]string{true: "[ Confirm ]", false: "[ Cancel ]"}[b.confirm]) {
					t.Fatal("hitbox/render mismatch", rows, b)
				}
			}
			// Clicking outside, scrolling, and releasing a button must do nothing.
			m.Update(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			m.Update(tea.MouseMsg{X: m.contentOffset() + 2, Y: buttons[0].y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
			if m.restartConfirm == nil || len(c.calls) != 0 {
				t.Fatal("unexpected effect")
			}
			b := buttons[1]
			if confirm {
				b = buttons[0]
			}
			_, cmd := m.Update(tea.MouseMsg{X: m.contentOffset() + b.x, Y: b.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if m.restartConfirm != nil || (cmd != nil) != confirm {
				t.Fatal("button did not activate")
			}
			if confirm {
				if msg := cmd().(restartFinished); msg.err != nil {
					t.Fatal(msg.err)
				}
			}
		}
	}
}
