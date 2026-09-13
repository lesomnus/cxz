package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/muesli/termenv"
)

func TestQuotaBarThresholds(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	now := time.Now()
	for _, tc := range []struct {
		value float64
		color string
	}{
		{0, "#F49BAA"}, {15, "#F49BAA"}, {15.1, "#F5AF98"},
		{30, "#F5AF98"}, {30.1, "#F5CA9A"}, {50, "#F5CA9A"},
	} {
		style := quotaBarStyle(tc.value)
		if style.GetForeground() != lipgloss.Color(tc.color) {
			t.Fatalf("%v: wrong color", tc.value)
		}
		m := model{quotaWindows: []agentview.Window{{Remaining: &tc.value, Label: "5h", Observed: now, Reset: now.Add(time.Hour)}}}
		got := m.quotaStatus(now, 100)
		bar := style.Render(quotaBar(tc.value))
		if !strings.Contains(got, bar) {
			t.Fatalf("bar color lost: %q", got)
		}
		plain := ansi.Strip(got)
		if !strings.HasSuffix(plain, " 5h 1h0m") || ansi.StringWidth(got) != len([]rune(plain)) {
			t.Fatalf("layout changed: %q", got)
		}
	}
	if quotaBarStyle(50.1).GetForeground() != muted.GetForeground() {
		t.Fatal("normal color changed")
	}
}

func TestKeyboardModeScreenLifecycle(t *testing.T) {
	var out bytes.Buffer
	w := &cursorWriter{out: &out, keyboard: true}
	for i := 0; i < 2; i++ { // Also covers tea.Exec release/restore.
		for _, s := range []string{"\x1b[?1049h", "frame", "\x1b[?1049l", "login"} {
			if n, err := w.Write([]byte(s)); err != nil || n != len(s) {
				t.Fatalf("write: %d %v", n, err)
			}
		}
	}
	want := strings.Repeat("\x1b[?1049h\x1b[>1u\x1b[?uframe\x1b[<u\x1b[?1049llogin", 2)
	if out.String() != want {
		t.Fatalf("mode order: %q", out.String())
	}
}
