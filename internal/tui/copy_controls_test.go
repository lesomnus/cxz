package tui

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

func TestMarkdownCodeCopyLayoutAndSource(t *testing.T) {
	profile := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	raw := "A literal ⧉ is not a button.\n\n- nested\n\n  ```go\n  package main\n  \tvar n = 1\n  ```\n\n> ```sh\n> echo 한글\n> ```"
	for _, color := range []termenv.Profile{termenv.Ascii, termenv.ANSI256, termenv.TrueColor} {
		lipgloss.SetColorProfile(color)
		for _, width := range []int{16, 40, 80} {
			view, buttons := markdownContent(raw, width)
			if strings.Contains(view, "cxz-copy") || len(buttons) != 2 {
				t.Fatalf("copy metadata: %d %s %q", width, color.Name(), view)
			}
			rows := strings.Split(view, "\n")
			for i, b := range buttons {
				if got := ansi.Strip(ansi.Cut(rows[b.y], b.x, b.x+3)); got != " ⧉ " {
					t.Fatalf("misaligned copy button: %+v %q", b, got)
				}
				want := []string{"package main\n\tvar n = 1\n", "echo 한글\n"}[i]
				if b.source != want {
					t.Fatalf("source was wrapped or indented: %q", b.source)
				}
			}
			for _, row := range rows {
				if ansi.StringWidth(row) > width {
					t.Fatalf("overflow: %q", row)
				}
			}
		}
	}
}

func TestConversationCodeCopyAfterCacheResizeAndScroll(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	for _, width := range []int{60, 110, 240} {
		m := conversationModel()
		var out bytes.Buffer
		m.cursorOutput = &cursorWriter{out: &out}
		source := "printf '한글'\n\tprintf done\n"
		e := &api.Event{Kind: "assistant", Seq: 1, Text: strings.Repeat("intro text\n", 25) + "\n```sh\n" + source + "```"}
		m.events["s"] = []*api.Event{e}
		page := m.prepareHistoryPage(context.Background(), historyPage{events: []*api.Event{e}}, "claude", m.view.Width)
		m.renderedResponses = page.prepared
		m.Update(tea.WindowSizeMsg{Width: width, Height: 32})
		m.render()
		if len(m.codeButtons) != 1 {
			t.Fatal("missing cached buttons", m.codeButtons)
		}
		b := m.codeButtons[0]
		m.view.SetYOffset(b.y - 3)
		x, y := m.contentOffset()+b.x+1, b.y-m.view.YOffset
		before := m.View()
		m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion})
		if out.Len() != 0 || before == m.View() {
			t.Fatal("hover must highlight without copying")
		}
		m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
		if out.Len() != 0 {
			t.Fatal("release copied")
		}
		m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if !strings.Contains(out.String(), ansi.SetSystemClipboard(source)) {
			t.Fatalf("wrong clipboard source: %q", out.String())
		}
		out.Reset()
		m.openReport("test", "visible overlay")
		m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if out.Len() != 0 {
			t.Fatal("copied through overlay")
		}
	}
}

func TestPreviewSixteenRowsAndCopyCloseButtons(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	for _, width := range []int{60, 110, 240} {
		for _, source := range []string{"one line", strings.Repeat("long\t한글\n", 40)} {
			m := conversationModel()
			m.Update(tea.WindowSizeMsg{Width: width, Height: 36})
			m.filePreview = &filePreview{session: "s", title: "Write · demo.go", source: source, language: "text", focused: true}
			m.resize()
			m.render()
			var out bytes.Buffer
			m.cursorOutput = &cursorWriter{out: &out}
			w, h := max(1, m.width-4), m.previewHeight()
			x, y := m.contentOffset()+2, m.view.Height+1+m.approvalHeight()
			if side := m.previewSideWidth(); side > 0 {
				w, h = side, m.height
				x, y = m.contentOffset()+m.width+2, 0
			}
			rows := strings.Split(ansi.Strip(m.previewRows(w, h)), "\n")
			if len(rows) != h || !strings.Contains(rows[0], "Write") || !strings.Contains(rows[h-2], "/") || !strings.Contains(rows[h-1], "─") {
				t.Fatal("fixed preview rows", rows)
			}
			if source == "one line" {
				for _, row := range rows[2 : h-2] {
					if strings.TrimSpace(row) != "" {
						t.Fatal("short source not padded", rows)
					}
				}
			}
			before := m.View()
			m.Update(tea.MouseMsg{X: x + w - 7, Y: y, Action: tea.MouseActionMotion})
			if m.filePreview.hover != "copy" || before == m.View() || out.Len() != 0 {
				t.Fatal("copy hover")
			}
			m.Update(tea.MouseMsg{X: x + w - 7, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if !strings.Contains(out.String(), ansi.SetSystemClipboard(source)) || m.filePreview == nil {
				t.Fatal("copy changed preview or source")
			}
			out.Reset()
			m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			if !strings.Contains(out.String(), ansi.SetSystemClipboard(source)) {
				t.Fatal("Ctrl+C did not copy preview")
			}
			m.Update(tea.MouseMsg{X: x + w - 3, Y: y, Action: tea.MouseActionMotion})
			if m.filePreview.hover != "close" {
				t.Fatal("close hover")
			}
			m.Update(tea.MouseMsg{X: x + w - 3, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if m.filePreview != nil {
				t.Fatal("close button did not close")
			}
		}
	}
}

func TestConversationSelectionCopyAndDetach(t *testing.T) {
	m := conversationModel()
	m.events["s"] = []*api.Event{{Kind: "assistant", Seq: 1, Text: "alpha 한글 beta\nsecond line"}}
	m.render()
	m.view.GotoTop()
	var out bytes.Buffer
	m.cursorOutput = &cursorWriter{out: &out}
	for _, v := range []tea.MouseMsg{
		{X: m.contentOffset() + 2, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress},
		{X: m.contentOffset() + 8, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion},
		{X: m.contentOffset() + 8, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease},
	} {
		m.Update(v)
	}
	m.View()
	// Animation elsewhere in the viewport must not discard selected text.
	m.current().State = "working"
	m.render()
	m.Update(pulseTick{})
	m.View()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil || !strings.Contains(out.String(), ansi.SetSystemClipboard("alpha 한글 beta\n  second")) {
		t.Fatalf("selection copy: %q", out.String())
	}
	m.current().Id = "other"
	out.Reset()
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if out.Len() != 0 {
		t.Fatal("selection crossed sessions")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Fatal("missing detach")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+D did not detach")
	}
}

func TestRecordingStatusPositionAndBlink(t *testing.T) {
	m := conversationModel()
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 36})
	m.toggleRecording()
	start := m.debugRecorder.generation()
	on := ansi.Strip(m.recordingLabel(start))
	off := ansi.Strip(m.recordingLabel(start.Add(time.Second)))
	if on != "⬤ REC " || strings.TrimSpace(off) != "REC" || strings.Contains(off, "⬤") || ansi.StringWidth(on) != ansi.StringWidth(off) || m.recordingLabel(start) != m.recordingLabel(start.Add(2*time.Second)) {
		t.Fatal("blink changes label/width", on, off)
	}
	for _, preview := range []bool{false, true} {
		if preview {
			m.filePreview = &filePreview{session: "s", source: "short"}
			m.resize()
			m.render()
		}
		rows := strings.Split(ansi.Strip(m.sessionScreen()), "\n")
		y := m.height - m.input.Height() - 4 - m.terminalHeight()
		if !strings.HasSuffix(rows[y], "REC ") || !strings.Contains(rows[y+1], "╭") {
			t.Fatal("recording not immediately above input", rows)
		}
		if strings.Contains(rows[len(rows)-1], "REC") {
			t.Fatal("recording overwrote quota")
		}
	}
	m.debugRecorder.Stop()
}

func TestProjectConnectionLabelDedupAndDim(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := conversationModel()
	m.client = &routingUIClient{}
	m.panelProjects = []*api.Project{{Id: "remote::p", Name: "x via remote", Alias: "x"}}
	m.allSessions = []*api.Session{}
	m.panelFocus = true
	m.panelIndex = 0
	view := m.panelScreen()
	row := strings.Split(ansi.Strip(view), "\n")[projectPanelHeaderRows]
	if !strings.Contains(row, "x via remote") || strings.Contains(row, "· x") {
		t.Fatal("duplicate alias", row)
	}
	terminal := vt.NewEmulator(m.panelScreenWidth(), m.height)
	defer terminal.Close()
	terminal.WriteString(strings.ReplaceAll(view, "\n", "\r\n"))
	name := terminal.CellAt(3, projectPanelHeaderRows)
	via := terminal.CellAt(5, projectPanelHeaderRows)
	if name == nil || via == nil || name.Style.Fg == nil || via.Style.Fg == nil || fmt.Sprint(name.Style.Fg) == fmt.Sprint(via.Style.Fg) {
		t.Fatal("connection label not dimmed")
	}
	r, g, b, _ := via.Style.Fg.RGBA()
	nr, ng, nb, _ := name.Style.Fg.RGBA()
	if r+g+b >= nr+ng+nb {
		t.Fatalf("connection is not darker: %x %x %x", r, g, b)
	}
}
