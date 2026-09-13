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

func TestTerminalToggleKittyAndPaste(t *testing.T) {
	r := keyboardReader{}
	out, pending := r.translate([]byte("\x1b[96;5u"), true)
	if string(out) != "\x1b[34~" || len(pending) != 0 {
		t.Fatalf("toggle lost: %q", out)
	}
	out, _ = r.translate([]byte("\x1b[200~\x1b[96;5u\x1b[201~"), true)
	if strings.Contains(string(out), "\x1b[34~") {
		t.Fatal("pasted shortcut was executed")
	}
}

func TestKittyUnicodeCommit(t *testing.T) {
	r := keyboardReader{}
	for _, input := range []string{"한글", "\x1b[54620u\x1b[44544u"} {
		out, pending := r.translate([]byte(input), true)
		if string(out) != "한글" || len(pending) != 0 {
			t.Fatalf("Unicode commit corrupted: %q", out)
		}
	}
}

func TestKittyBackwardWord(t *testing.T) {
	r := keyboardReader{}
	out, _ := r.translate([]byte("\x1b[127;5u"), true)
	if string(out) != "\x17" {
		t.Fatalf("Ctrl+Backspace: %q", out)
	}
	out, _ = r.translate([]byte("\x1b[200~\x1b[127;5u\x1b[201~"), true)
	if bytes.Contains(out, []byte{23}) {
		t.Fatal("interpreted pasted word deletion")
	}
}

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
		if !strings.HasSuffix(plain, " 5h 1h") || ansi.StringWidth(got) != len([]rune(plain)) {
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
