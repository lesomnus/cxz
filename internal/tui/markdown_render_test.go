package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

func TestMarkdownListHangingIndent(t *testing.T) {
	profile := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	for _, profile := range []termenv.Profile{termenv.Ascii, termenv.ANSI256, termenv.TrueColor} {
		lipgloss.SetColorProfile(profile)
		for _, tc := range []struct {
			source string
			width  int
			want   string
		}{
			{"- foo abcdefghi\n- baz", 10, "• foo abcd\n  efghi\n• baz"},
			{"- foo\n  bar\n- baz", 10, "• foo\n  bar\n• baz"},
			{"9. abcdefghijk\n10. bazxyz", 10, "9. abcdefg\n   hijk\n10. bazxyz"},
			{"- parent\n  - abcdefghijk\n- next", 10, "• parent\n  • abcdef\n    ghijk\n• next"},
			{"- **한글한글한글**", 10, "• 한글한글\n  한글"},
			{"> - abcdefghijk", 10, "│ • abcdef\n│   ghijk"},
		} {
			view := markdownView(tc.source, tc.width)
			plainRows := strings.Split(ansi.Strip(view), "\n")
			for i := range plainRows {
				plainRows[i] = strings.TrimRight(plainRows[i], " ")
			}
			if got := strings.Join(plainRows, "\n"); got != tc.want {
				t.Fatalf("%s: %q at width %d\ngot  %q\nwant %q", profile.Name(), tc.source, tc.width, got, tc.want)
			}
			for _, row := range strings.Split(view, "\n") {
				if ansi.StringWidth(row) > tc.width {
					t.Fatal("list overflow", row)
				}
			}
		}
	}
}

func TestMarkdownDetectedSyntaxAndIndexedBackground(t *testing.T) {
	old := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(old)
	for _, profile := range []termenv.Profile{termenv.ANSI256, termenv.TrueColor} {
		lipgloss.SetColorProfile(profile)
		for _, language := range []string{"python", ""} {
			view := markdownView("```"+language+"\n#!/usr/bin/env python3\ndef greet(name):\n    return \"Hello\"\n```", 40)
			if !strings.Contains(view, "38;") || !strings.Contains(view, fmt.Sprintf("48;5;%d", codeBackground)) {
				t.Fatal("highlight missing", view)
			}
			if profile == termenv.ANSI256 && strings.Contains(view, "38;2;") {
				t.Fatal("truecolor on ANSI256")
			}
			terminal := vt.NewEmulator(40, 6)
			terminal.WriteString(strings.ReplaceAll(view, "\n", "\r\n"))
			foregrounds := map[string]bool{}
			for y := 0; y < 5; y++ {
				for x := 0; x < 40; x++ {
					cell := terminal.CellAt(x, y)
					if cell != nil && cell.Width == 0 {
						continue
					}
					if cell == nil || cell.Style.Bg == nil {
						t.Fatalf("unfilled code cell %d,%d", x, y)
					}
					if cell.Style.Fg != nil {
						foregrounds[fmt.Sprint(cell.Style.Fg)] = true
					}
					r, g, b, _ := cell.Style.Bg.RGBA()
					gray := uint32(8+10*(codeBackground-232)) * 0x101
					if r != gray || g != r || b != r {
						t.Fatalf("code background %04x %04x %04x, want %04x", r, g, b, gray)
					}
				}
			}
			terminal.Close()
			if len(foregrounds) < 2 {
				t.Fatal("code was not syntax highlighted", language, view)
			}
		}
	}
}

func TestCodeLanguageDetection(t *testing.T) {
	for source, want := range map[string]string{
		"def greet(name):\n    return name":  "python",
		"package main\nfunc main() {}":       "go",
		"docker compose up -d":               "bash",
		`{"count": 2}`:                       "json",
		"const value = 2;":                   "javascript",
		"interface Item { value: number }":   "typescript",
		"services:\n  app:\n    image: demo": "yaml",
		"A normal sentence without code.":    "plaintext",
	} {
		if got := detectCodeLanguage(source); got != want {
			t.Fatalf("%q: got %s, want %s", source, got, want)
		}
	}
}

func TestMarkdownTableBordersAndWrapping(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(old)
	raw := "| Name | Count |\n| :--- | ---: |\n| 한글 long value | 123 |\n| **other** | 4 |"
	for _, width := range []int{8, 20, 60} {
		view := markdownView(raw, width)
		plain := ansi.Strip(view)
		if strings.ContainsAny(plain, "│┌┐└┘|") || !strings.Contains(view, "38;5;22m") || !strings.Contains(plain, "━ ━") {
			t.Fatal("table borders", view)
		}
		if !strings.Contains(view, "38;5;240m") || !strings.Contains(plain, "─ ─") || strings.Count(plain, "─") != strings.Count(plain, "━") {
			t.Fatal("data rows need one thin gray divider, matching the header columns", view)
		}
		for _, row := range strings.Split(view, "\n") {
			if ansi.StringWidth(row) > width {
				t.Fatal("table overflow", row)
			}
		}
		if !strings.Contains(plain, "123") || (!strings.Contains(plain, "한") || !strings.Contains(plain, "글")) {
			t.Fatal("cell lost", plain)
		}
	}
}

func TestUserMessageBackgroundReachesConversationEdges(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(old)
	for _, width := range []int{40, 100, 200} {
		m := conversationModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		m.events["s"] = []*api.Event{{Seq: 1, Kind: "input", Text: "한글 prompt\nsecond line"}, {Seq: 2, Kind: "assistant", Text: "answer"}}
		m.render()
		m.view.GotoTop()
		terminal := vt.NewEmulator(m.width, 24)
		terminal.WriteString(strings.ReplaceAll(m.conversationView(), "\n", "\r\n"))
		// Top padding, timestamp, two input rows, and bottom padding share the fill.
		for y := 0; y <= 4; y++ {
			for x := 0; x < m.width; x++ {
				cell := terminal.CellAt(x, y)
				if cell != nil && cell.Width == 0 {
					continue
				}
				if cell == nil || cell.Style.Bg == nil {
					t.Fatalf("unfilled user cell %d,%d width=%d", x, y, width)
				}
				// xterm's grayscale ramp: index 232+n is the gray 8+10n.
				gray := uint32(8+10*(promptBackground-232)) * 0x101
				r, g, b, _ := cell.Style.Bg.RGBA()
				if r != gray || g != r || b != r {
					t.Fatalf("wrong user background %04x, want %04x", r, gray)
				}
			}
		}
		if cell := terminal.CellAt(0, 5); cell != nil && cell.Style.Bg != nil {
			t.Fatal("user background leaked past bottom padding")
		}
		terminal.Close()
	}
}

// The fill reaches the edge of the conversation, so a cursor drawn against it
// has nothing between the two. One column of fill stays to its left, and that
// column belongs to the message rather than to the gap beside the panel.
func TestPromptCursorKeepsAColumnOfFillToItsLeft(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(old)
	m := conversationModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m.events["s"] = []*api.Event{{Seq: 1, Kind: "input", Text: "hello"}, {Seq: 2, Kind: "assistant", Text: "world"}}
	m.render()
	m.view.GotoTop()
	if m.contentOffset() == 0 {
		t.Fatal("fixture needs a visible panel")
	}
	terminal := vt.NewEmulator(m.terminalWidth, 20)
	terminal.WriteString(strings.ReplaceAll(m.View(), "\n", "\r\n"))
	filled := func(x, y int) bool {
		cell := terminal.CellAt(x, y)
		if cell == nil || cell.Style.Bg == nil {
			return false
		}
		r, _, _, _ := cell.Style.Bg.RGBA()
		return r == uint32(8+10*(promptBackground-232))*0x101
	}
	at := func(x, y int) string {
		if cell := terminal.CellAt(x, y); cell != nil {
			return cell.Content
		}
		return ""
	}
	edge := m.contentOffset()
	cursor := -1
	for y := 0; y < 8 && cursor < 0; y++ {
		if at(edge+promptIndent, y) == "❯" {
			cursor = y
		}
	}
	if cursor < 0 {
		t.Fatal("cursor is not one column inside the conversation edge")
	}
	if !filled(edge, cursor) || at(edge, cursor) != " " {
		t.Fatalf("column left of the cursor is not blank fill: %q", at(edge, cursor))
	}
	// The block did not move outward to make room: the gap beside the panel is
	// untouched, and the fill still starts where the conversation does.
	if filled(edge-1, cursor) {
		t.Fatal("fill reached into the gap beside the panel")
	}
}
