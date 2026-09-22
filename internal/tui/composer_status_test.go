package tui

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

func TestPermissionLabelStaysInFooter(t *testing.T) {
	for _, width := range []int{40, 110, 240} {
		for _, state := range []string{"working", "stopped"} {
			m := conversationModel()
			m.Update(tea.WindowSizeMsg{Width: width, Height: 36})
			m.current().PermissionMode = "full"
			m.current().State = state
			m.events["s"] = []*api.Event{{Seq: 1, Kind: "assistant", Text: strings.Repeat("history\n", 60)}}
			m.render()
			for _, bottom := range []bool{true, false} {
				if bottom {
					m.view.GotoBottom()
				} else {
					m.view.GotoTop()
				}
				rows := strings.Split(ansi.Strip(m.sessionScreen()), "\n")
				footer := rows[len(rows)-1]
				if !strings.HasPrefix(footer, " FULL ") || ansi.StringWidth(footer) > m.width || strings.Count(strings.Join(rows, "\n"), "FULL") != 1 {
					t.Fatal("permission label missing, duplicated, or displaced quota", rows)
				}
				if strings.Contains(strings.Join(rows, "\n"), "background approval") || strings.Contains(strings.Join(rows, "\n"), "/permission ask") {
					t.Fatal("verbose permission hint remains")
				}
			}
			m.current().PermissionMode = "ask"
			if strings.Contains(m.sessionScreen(), "FULL") {
				t.Fatal("FULL remained after switching to ask")
			}
		}
	}
}

func TestStatusClickCopiesWholeVisibleNotice(t *testing.T) {
	for _, width := range []int{40, 110, 240} {
		for _, preview := range []bool{false, true} {
			m := conversationModel()
			m.Update(tea.WindowSizeMsg{Width: width, Height: 36})
			if preview {
				m.filePreview = &filePreview{session: "s", title: "Read", source: "file", focused: true}
				m.resize()
				m.render()
			}
			m.current().State = "stopped"
			m.toggleRecording()
			message := "rpc error: " + strings.Repeat("긴 오류 메시지 ", 40) + "\ncomplete second line: detail beyond the screen"
			m.notice = message
			var out bytes.Buffer
			m.cursorOutput = &cursorWriter{out: &out}
			rows := strings.Split(ansi.Strip(m.sessionScreen()), "\n")
			y := m.height - m.input.Height() - m.terminalHeight() - 4
			index := strings.Index(rows[y], "rpc error:")
			if index < 0 || strings.Contains(rows[y], "beyond the screen") {
				t.Fatal("test needs a visible clipped error", rows[y])
			}
			x := m.contentOffset() + ansi.StringWidth(rows[y][:index]) + 1
			if m.panelVisible() && strings.Contains(m.panelScreen(), "rpc error:") {
				t.Fatal("sidebar duplicates composer error")
			}
			// Hover, release, state prefix, and REC are not clipboard actions.
			for _, event := range []tea.MouseMsg{
				{X: x, Y: y, Action: tea.MouseActionMotion},
				{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease},
				{X: m.contentOffset() + 2, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress},
				{X: m.contentOffset() + m.width - 2, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress},
			} {
				m.Update(event)
			}
			if out.Len() != 0 {
				t.Fatal("non-notice copied", out.String())
			}
			m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if !strings.Contains(out.String(), ansi.SetSystemClipboard(message)) {
				t.Fatalf("copied clipped text instead of full notice: %q", out.String())
			}
			if len(m.noticeLogs) == 0 || m.noticeLogs[0].text != message {
				t.Fatal("copy feedback lost original notice log")
			}
			out.Reset()
			m.notice = message
			m.events["s"] = []*api.Event{{Seq: 1, Kind: "assistant", Text: strings.Repeat("history\n", 100)}}
			m.render()
			m.view.GotoTop()
			m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if out.Len() != 0 {
				t.Fatal("scroll hint copied a hidden notice")
			}
			m.debugRecorder.Stop()
		}
	}
}

func TestWidePreviewUsesFullHeightAndScrollPage(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	for _, height := range []int{24, 50} {
		m := conversationModel()
		m.Update(tea.WindowSizeMsg{Width: 240, Height: height})
		m.filePreview = &filePreview{session: "s", title: "Read", source: "one line", focused: true}
		m.resize()
		m.render()
		x := m.contentOffset() + m.width + 2
		terminal := vt.NewEmulator(240, height)
		terminal.WriteString(strings.ReplaceAll(m.View(), "\n", "\r\n"))
		for y := 0; y < height; y++ {
			for column := x; column < x+m.previewSideWidth(); column++ {
				cell := terminal.CellAt(column, y)
				if cell == nil || cell.Style.Bg == nil {
					t.Fatal("unfilled right panel", column, y)
				}
			}
		}
		terminal.Close()
		m.filePreview.source = strings.Repeat("source\n", 150)
		m.filePreview.rendered = ""
		m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		if m.filePreview.offset != height-previewFrameRows {
			t.Fatal("wrong page size", m.filePreview.offset)
		}
		m.Update(tea.MouseMsg{X: x + 2, Y: height - 4, Button: tea.MouseButtonWheelDown})
		if m.filePreview.offset != height-previewFrameRows+3 {
			t.Fatal("lower part of sidebar ignored mouse wheel")
		}
		m.Update(tea.WindowSizeMsg{Width: 110, Height: 36})
		if m.previewHeight() != previewContentRows+previewFrameRows {
			t.Fatal("inline preview lost fixed height")
		}
	}
}
