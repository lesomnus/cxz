package tui

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestTerminalPaletteNavigationResizeAndClick(t *testing.T) {
	m := newTerminalInfo(strings.NewReader(""), &bytes.Buffer{})
	m.Update(tea.WindowSizeMsg{Width: 96, Height: 24})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.selected != 17 {
		t.Fatal(m.selected)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if m.selected != 255 || m.firstRow == 0 {
		t.Fatal("last color not revealed")
	}
	m.Update(tea.MouseMsg{X: 2*paletteCell + 2, Y: paletteTop + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if want := (m.firstRow+1)*16 + 2; m.selected != want {
		t.Fatalf("click=%d want=%d", m.selected, want)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !m.rgb || !strings.Contains(m.View(), "48;2;") || !strings.Contains(m.View(), paletteHex(m.selected)) {
		t.Fatal("RGB selection missing")
	}
	for _, size := range [][2]int{{24, 14}, {80, 24}, {160, 80}, {30, 18}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.Update(tea.KeyMsg{Type: tea.KeyEnd})
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) > size[1] {
			t.Fatalf("height overflow: %v, %d", size, len(lines))
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("width overflow", size, line)
			}
		}
		if !strings.Contains(view, "#EEEEEE") {
			t.Fatal("selected code clipped", size)
		}
		cols, _ := m.layout()
		y := paletteTop + m.selected/cols - m.firstRow
		x := m.selected % cols * paletteCell
		if !strings.Contains(ansi.Strip(lines[y]), "255") {
			t.Fatal("selected row not visible")
		}
		m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.selected != 255 {
			t.Fatal("resize broke mouse coordinates")
		}
	}
}

func TestTerminalPaletteReferenceAndPlainReport(t *testing.T) {
	for index, want := range map[int]string{0: "#000000", 15: "#FFFFFF", 16: "#000000", 17: "#00005F", 231: "#FFFFFF", 232: "#080808", 235: "#262626", 255: "#EEEEEE"} {
		if got := paletteHex(index); got != want {
			t.Fatalf("%d: %s != %s", index, got, want)
		}
	}
	t.Setenv("TERM_PROGRAM", "sample\n\x1b[31mterminal")
	var out bytes.Buffer
	if err := RunTerminalInfo(t.Context(), strings.NewReader(""), &out, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b") || !strings.Contains(out.String(), "255 #EEEEEE") {
		t.Fatal("bad plain output")
	}
	m := newTerminalInfo(strings.NewReader(""), &out)
	if len(m.header()) != paletteTop {
		t.Fatal("environment broke grid origin")
	}
}
