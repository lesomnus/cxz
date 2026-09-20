package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func TestMultilineComposerAndDraft(t *testing.T) {
	m := projectModel()
	c := &recordingClient{}
	m.client = c
	m.input = newComposer()
	m.sessions = []*api.Session{{Id: "s", ProjectId: "p"}}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// A bracketed paste is one rune event, even when it contains newlines.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("한글\ncode"), Paste: true})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("last")})
	if len(c.inputs) != 0 || m.input.Value() != "한글\ncode\nlast" {
		t.Fatalf("paste/newline: %q", m.input.Value())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.input.Value() != "한글\ncode\nlast" {
		t.Fatal("draft lost")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	cmd()
	if len(c.inputs) != 1 || c.inputs[0].Text != "한글\ncode\nlast" {
		t.Fatal("multiline send changed")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("draft")})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.input.Value() != "draft" {
		t.Fatal("Esc discarded draft")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if m.input.Value() != "" {
		t.Fatal("clear failed")
	}
}

func TestScreenDisplayBounds(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {80, 24}, {40, 16}, {20, 10}} {
		m := projectModel()
		m.input = newComposer()
		m.project.Name = strings.Repeat("프로젝트", 30) + "\x1b]52;c;secret\a"
		for i := 0; i < 20; i++ {
			m.sessions = append(m.sessions, &api.Session{Id: "session", Title: strings.Repeat("대화", 30), ProjectId: "p", Agent: "codex", State: "stopped"})
		}
		for _, project := range []bool{true, false} {
			m.projectView = project
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			view := m.View()
			if strings.Contains(view, "secret") {
				t.Fatal("unsafe escape payload")
			}
			lines := strings.Split(view, "\n")
			if len(lines) > size[1] {
				t.Fatalf("height %d > %d", len(lines), size[1])
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("width overflow: %q", line)
				}
			}
		}
	}
}

func TestScreenPreview(t *testing.T) {
	m := projectModel()
	m.input = newComposer()
	m.project.Name = "cxz workspace"
	m.project.Alias = "cxz"
	m.sessions = []*api.Session{{Id: "a1b2c3d4", ProjectId: "p", Title: "Polish the project dashboard", Agent: "codex", Account: "work", State: "running"}}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	t.Log("\n" + m.View())
	m.projectView = false
	m.events["a1b2c3d4"] = []*api.Event{{Kind: "input", Text: "프로젝트 화면을 정리해줘."}, {Kind: "assistant", Text: "프로젝트 정보와 대화 목록을 분리하겠습니다.\n\n- 선택한 세션을 강조합니다.\n- 입력창은 하단에 고정합니다."}}
	m.render()
	t.Log("\n" + m.View())
}
