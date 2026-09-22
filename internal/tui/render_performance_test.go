package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
)

func BenchmarkLongPaste(b *testing.B) {
	body := []rune(strings.Repeat("한글 text ", 512))
	for _, atomic := range []bool{false, true} {
		name := "individual_keys"
		if atomic {
			name = "bracketed_paste"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(string(body))))
			for b.Loop() {
				m := conversationModel()
				if atomic {
					m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: body, Paste: true})
					m.View()
				} else {
					for _, char := range body {
						m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
						m.View()
					}
				}
			}
		})
	}
}

func BenchmarkConversationFrame(b *testing.B) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	for _, size := range []int{0, 256 * 1024} {
		name := "small"
		if size > 0 {
			name = "large_result"
		}
		b.Run(name, func(b *testing.B) {
			m := conversationModel()
			m.cursorOutput = &cursorWriter{out: io.Discard}
			payload, _ := json.Marshal(map[string]any{"type": "result", "result": strings.Repeat("x", size), "modelUsage": map[string]any{"claude": map[string]any{"contextWindow": 200000}}})
			m.events["s"] = []*api.Event{
				{Seq: 1, RunId: "run", Kind: "input", Text: "Explain this code"},
				{Seq: 2, RunId: "run", Kind: "usage", Text: "context/message", Payload: []byte(`{"model":"claude","usage":{"input_tokens":1500}}`)},
				{Seq: 3, RunId: "run", Kind: "assistant", Text: "Here is the explanation.\n\n- First point\n- Second point"},
				{Seq: 4, RunId: "run", Kind: "turn_end", Payload: payload},
			}
			m.render()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				m.View()
			}
		})
	}
}

func BenchmarkLiveTranscriptRender(b *testing.B) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	for _, turns := range []int{32, 128} {
		b.Run(fmt.Sprint(turns), func(b *testing.B) {
			m := conversationModel()
			m.current().State = "working"
			payload, _ := json.Marshal(map[string]any{"result": strings.Repeat("past result ", 2000), "duration_ms": 1234, "usage": map[string]int{"input_tokens": 1500, "output_tokens": 800}, "total_cost_usd": 0.2})
			for i := 0; i < turns; i++ {
				m.events["s"] = append(m.events["s"],
					&api.Event{Seq: uint64(i*3 + 1), RunId: "run", Kind: "input", Text: "Explain this code", TimeMs: int64(i*2000 + 1)},
					&api.Event{Seq: uint64(i*3 + 2), RunId: "run", Kind: "assistant", Text: "Here is the explanation.\n\n- First point\n- Second point"},
					&api.Event{Seq: uint64(i*3 + 3), RunId: "run", Kind: "turn_end", Text: "completed", Payload: payload, TimeMs: int64(i*2000 + 1235)})
			}
			m.render()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				m.render()
			}
		})

	}
}
