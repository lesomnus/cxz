//go:build unix

package tui

import (
	"bytes"
	"strings"
	"testing"
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
