package supervisor

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func assistantLine(t *testing.T, text string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"model": "claude-test", "content": []any{map[string]any{"type": "text", "text": text}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A reply far past anything a reader expects is still an ordinary reply, and
// gets journaled whole. Only a malfunction reaches the bound, so a message that
// merely overshoots an expectation must never be cut or dropped.
func TestLongReplyIsDeliveredWhole(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	text := strings.Repeat("긴 응답 ", 1<<16) // ~700 KiB, under the bound
	line := assistantLine(t, text)
	if len(line) >= MaxLineBytes {
		t.Fatal("fixture is not under the bound:", len(line))
	}
	tail := `{"type":"result","subtype":"success"}`
	if err := s.read(strings.NewReader(line + "\n" + tail)); err != nil {
		t.Fatal(err)
	}
	var delivered, raws int
	for _, e := range s.log.All() {
		if e.Kind == "assistant" {
			if e.Text != text {
				t.Fatal("assistant text altered:", len(e.Text), "of", len(text))
			}
			delivered++
		}
		// The line terminator is not part of the vendor bytes, and a final line
		// without one is a complete record too.
		if e.Kind == "raw" {
			if e.Raw[len(e.Raw)-1] == '\n' {
				t.Fatal("raw event kept its line terminator")
			}
			raws++
		}
	}
	if delivered != 1 || raws != 2 {
		t.Fatal("delivered", delivered, "assistant and", raws, "raw events")
	}
}

// Past the bound the stream is abandoned, but what arrived is kept and the
// truncation is stated. The old reader discarded the line and killed the agent,
// so the session ended with no record of what it had already received.
func TestOverlongLineIsRecordedAndReported(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	line := assistantLine(t, strings.Repeat("x", 2*MaxLineBytes))
	err := s.read(strings.NewReader(line + "\n" + assistantLine(t, "never read")))
	if err == nil {
		t.Fatal("overflow was not reported")
	}
	if !strings.Contains(err.Error(), "stopped reading") {
		t.Fatal("unhelpful overflow error:", err)
	}
	var fragment, marked bool
	for _, e := range s.log.All() {
		switch e.Kind {
		case "raw":
			if len(e.Raw) != MaxLineBytes {
				t.Fatal("fragment of", len(e.Raw), "bytes, wanted the whole budget")
			}
			if !strings.HasPrefix(line, string(e.Raw)) {
				t.Fatal("fragment is not the start of the message")
			}
			fragment = true
		case "diagnostic":
			marked = strings.Contains(e.Text, "exceeded") && strings.Contains(e.Text, "recorded")
		case "assistant":
			t.Fatal("projected an incomplete message")
		}
	}
	if !fragment || !marked {
		t.Fatal("fragment recorded:", fragment, "truncation marked:", marked)
	}
}

// A read failure is still fatal: the stream is desynchronized and the agent can
// no longer be spoken to, which is a different thing from a long message.
func TestReadErrorIsReported(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	if err := s.read(io.MultiReader(strings.NewReader("{\"type\":\"system\"}\n"), errReader{})); err == nil {
		t.Fatal("read failure was swallowed")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
