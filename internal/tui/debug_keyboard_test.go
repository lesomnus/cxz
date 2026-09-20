//go:build unix

package tui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKittyWordNavigationAndSafeRawTrace(t *testing.T) {
	recorder := &debugRecorder{}
	recorder.Start()
	r := keyboardReader{recorder: recorder}
	for input, want := range map[string]string{"\x1b[57350;5u": "\x1b[1;5D", "\x1b[57351;5u": "\x1b[1;5C", "\x1b[57417;5u": "\x1b[1;5D", "\x1b[1;5C": "\x1b[1;5C"} {
		for split := 1; split < len(input); split++ {
			first, pending := r.translate([]byte(input[:split]), false)
			next, _ := r.translate(append(pending, []byte(input[split:])...), true)
			if string(append(first, next...)) != want {
				t.Fatal("split navigation lost", split, input)
			}
		}
	}
	r.translate([]byte("PASSWORD\x1b[115u\x1b[200~PASTED\x1b[1;5D\x1b[201~"), true)
	a := recorder.Stop()
	b, _ := json.Marshal(a.Events)
	if strings.Contains(string(b), "PASSWORD") || strings.Contains(string(b), "115u") || strings.Contains(string(b), "PASTED") {
		t.Fatal("raw input leaked", string(b))
	}
	if len(a.Events) == 0 {
		t.Fatal("no navigation captured")
	}
	for _, e := range a.Events {
		if e.Kind != "terminal_navigation" && e.Kind != "terminal_protocol" {
			t.Fatal(e)
		}
	}
}
