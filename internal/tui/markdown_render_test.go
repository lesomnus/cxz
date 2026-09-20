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

func TestMarkdownDetectedSyntaxAndIndexedBackground(t *testing.T) {
	old := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(old)
	for _, profile := range []termenv.Profile{termenv.ANSI256, termenv.TrueColor} {
		lipgloss.SetColorProfile(profile)
		for _, language := range []string{"python", ""} {
			view := markdownView("```"+language+"\n#!/usr/bin/env python3\ndef greet(name):\n    return \"Hello\"\n```", 40)
			if !strings.Contains(view, "38;") || !strings.Contains(view, "48;5;238") {
				t.Fatal("highlight missing", view)
			}
			if profile == termenv.ANSI256 && strings.Contains(view, "38;2;") {
				t.Fatal("truecolor on ANSI256")
			}
			terminal := vt.NewEmulator(40, 5)
			terminal.WriteString(strings.ReplaceAll(view, "\n", "\r\n"))
			foregrounds := map[string]bool{}
			for y := 0; y < 3; y++ {
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
					if r != 0x4444 || g != r || b != r {
						t.Fatalf("code background %x %x %x", r, g, b)
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
		for y := 1; y <= 2; y++ {
			for x := 0; x < m.width; x++ {
				cell := terminal.CellAt(x, y)
				if cell != nil && cell.Width == 0 {
					continue
				}
				if cell == nil || cell.Style.Bg == nil {
					t.Fatalf("unfilled user cell %d,%d width=%d", x, y, width)
				}
				r, g, b, _ := cell.Style.Bg.RGBA()
				if r != 0x3030 || g != r || b != r {
					t.Fatal("wrong user background")
				}
			}
		}
		terminal.Close()
	}
}
