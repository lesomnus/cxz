package tui

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

func TestErrorDialogGeometryAndClipboard(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	for _, width := range []int{40, 110, 240} {
		for _, height := range []int{14, 36} {
			m := conversationModel()
			m.Update(tea.WindowSizeMsg{Width: width, Height: height})
			m.input.SetValue("keep draft")
			m.filePreview = &filePreview{session: "s", title: "Read", source: "file\ncontent"}
			message := "rpc error: " + strings.Repeat("긴 오류 메시지\t", 30) + "\ncomplete final detail"
			m.Update(result{err: errors.New(message)})
			if m.errorDialog == nil || m.errorFocused() || !m.input.Focused() || m.input.Value() != "keep draft" {
				t.Fatal("error stole input focus or draft")
			}
			x, y, w := m.errorBounds()
			view := m.View()
			rows := strings.Split(ansi.Strip(view), "\n")
			if len(rows) != height || !strings.Contains(rows[y], "Error") || !strings.Contains(rows[y+1], "rpc error:") || strings.Count(ansi.Strip(view), "rpc error:") != 1 {
				t.Fatalf("wrong dialog geometry %dx%d: %s", width, height, ansi.Strip(view))
			}
			for _, row := range rows {
				if ansi.StringWidth(row) != width {
					t.Fatal("line overflow", width, height, row)
				}
			}
			if !strings.Contains(rows[height-2], "╰") {
				t.Fatal("dialog displaced footer")
			}
			terminal := vt.NewEmulator(width, height)
			terminal.WriteString(strings.ReplaceAll(view, "\n", "\r\n"))
			for row := y; row < y+errorDialogRows; row++ {
				for col := x; col < x+w; col++ {
					cell := terminal.CellAt(col, row)
					if cell != nil && cell.Width == 0 && col > x {
						// VT represents the second half of a wide glyph as an
						// empty cell; the leading glyph owns its style.
						cell = terminal.CellAt(col-1, row)
					}
					if cell == nil || cell.Style.Bg == nil {
						t.Fatal("error background does not fill row", width, height, col, row)
					}
				}
			}
			terminal.Close()
			var out bytes.Buffer
			m.cursorOutput = &cursorWriter{out: &out}
			m.Update(tea.MouseMsg{X: x + w - 7, Y: y, Action: tea.MouseActionMotion})
			if m.errorDialog.hover != "copy" || out.Len() != 0 {
				t.Fatal("hover copied or failed to highlight")
			}
			m.Update(tea.MouseMsg{X: x + w - 7, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if !strings.Contains(out.String(), ansi.SetSystemClipboard(message)) || m.errorDialog == nil {
				t.Fatal("copy lost full error or closed dialog")
			}
			m.Update(tea.MouseMsg{X: x + 2, Y: y + 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if !m.errorFocused() || m.input.Focused() {
				t.Fatal("dialog focus did not blur input")
			}
			out.Reset()
			m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			if !strings.Contains(out.String(), ansi.SetSystemClipboard(message)) {
				t.Fatal("keyboard copy did not use full error")
			}
			m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
			if m.errorDialog.offset != 3 {
				t.Fatal("wrong page size")
			}
			m.Update(tea.KeyMsg{Type: tea.KeyEnd})
			if !strings.Contains(m.View(), "complete final detail") {
				t.Fatal("end of error inaccessible")
			}
			m.Update(tea.KeyMsg{Type: tea.KeyHome})
			m.Update(tea.MouseMsg{X: x + 2, Y: y + 2, Button: tea.MouseButtonWheelDown})
			if m.errorDialog.offset != 3 {
				t.Fatal("wheel did not scroll dialog")
			}
			m.Update(tea.KeyMsg{Type: tea.KeyTab})
			if m.errorFocused() || !m.input.Focused() || m.errorDialog == nil {
				t.Fatal("Tab must return focus without closing")
			}
			m.Update(tea.MouseMsg{X: x + w - 3, Y: y, Action: tea.MouseActionMotion})
			if m.errorDialog.hover != "close" {
				t.Fatal("missing close hover")
			}
			m.Update(tea.MouseMsg{X: x + w - 3, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if m.errorDialog != nil || m.input.Value() != "keep draft" || m.noticeLogs[0].text != message {
				t.Fatal("close destroyed draft or diagnostic log")
			}
		}
	}
}

func TestErrorDialogFixedContentAndReceiptIsolation(t *testing.T) {
	m := conversationModel()
	m.current().PermissionMode = "full"
	m.showError("one short error")
	rows := strings.Split(ansi.Strip(m.errorRows(40)), "\n")
	if len(rows) != errorDialogRows || strings.TrimSpace(rows[2]) != "" || strings.TrimSpace(rows[3]) != "" {
		t.Fatal("short error did not keep three content rows", rows)
	}
	m.Update(result{text: "send · accepted"})
	if m.errorDialog == nil || !strings.Contains(m.View(), "send · accepted") {
		t.Fatal("receipt replaced failure")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !m.errorFocused() {
		t.Fatal("keyboard cannot reach error")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}, Paste: true})
	if m.errorDialog == nil {
		t.Fatal("pasted x closed error")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.errorDialog != nil || !m.input.Focused() || !strings.Contains(m.View(), "send · accepted") {
		t.Fatal("closing error lost receipt or input")
	}
	m.showError("another failure")
	m.events["s"] = []*api.Event{{Seq: 1, Kind: "assistant", Text: strings.Repeat("history\n", 100)}}
	m.render()
	m.view.GotoTop()
	if !strings.Contains(m.View(), "another failure") {
		t.Fatal("conversation scroll hides failure")
	}
}

func TestErrorDialogWithApprovalsAndProjectView(t *testing.T) {
	for _, width := range []int{40, 110} {
		m := conversationModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 14})
		m.current().Pending = []*api.Event{{RunId: "run", Kind: "approval", RequestId: "a", Text: "Bash", Payload: []byte(`{}`)}}
		m.current().PermissionMode = "ask"
		m.input.SetValue("one\ntwo\nthree\nfour\nfive\nsix")
		m.showError("approval failure")
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		if len(rows) != 14 || !strings.Contains(rows[9], "╭") || !strings.Contains(rows[12], "╰") {
			t.Fatal("approval/error panels displaced composer", rows)
		}
		m.backToProject()
		m.showError("project operation failed")
		m.Update(tea.KeyMsg{Type: tea.KeyTab})
		if !m.errorFocused() {
			t.Fatal("project errors inaccessible by keyboard")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
		if m.errorDialog != nil || !m.panelFocus {
			t.Fatal("close did not return to project panel")
		}
	}
}

func TestErrorDialogResizeAndTerminal(t *testing.T) {
	m := conversationModel()
	m.current().PermissionMode = "full"
	m.terminals = map[string]*terminalPanel{"s": {open: true}}
	m.filePreview = &filePreview{session: "s", title: "Read", source: "source"}
	m.showError(strings.Repeat("long error\n", 60))
	m.focusError()
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	for _, size := range [][2]int{{240, 50}, {40, 14}, {110, 36}, {240, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		if len(rows) != size[1] || !strings.Contains(rows[size[1]-1], "FULL") {
			t.Fatal("error + terminal + preview displaced footer", size, rows)
		}
		x, y, width := m.errorBounds()
		if !strings.Contains(rows[y], "Error") || !strings.Contains(rows[y], "[×]") {
			t.Fatal("error buttons lost on resize", size, rows[y])
		}
		m.Update(tea.MouseMsg{X: x + width - 7, Y: y, Action: tea.MouseActionMotion})
		if m.errorDialog.hover != "copy" {
			t.Fatal("resized button target is stale")
		}
		m.Update(tea.MouseMsg{X: m.contentOffset() + 5, Y: size[1] - m.terminalHeight() - 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.errorFocused() || !m.input.Focused() {
			t.Fatal("clicking input did not return focus")
		}
		m.focusError()
	}
}
