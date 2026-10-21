package logview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

// A diagnostic report says what went wrong, never what was said: the
// conversation and the verbatim vendor payload stay out of it.
func TestTheDiagnosticReportOmitsTheConversation(t *testing.T) {
	var r Report
	r.Conversation("conversation", []core.Event{
		{Kind: "assistant", Text: "private conversation", Raw: []byte("raw vendor data")},
		{Kind: "update", Text: "update completed"},
		{Kind: "diagnostic", Text: "failure details"},
	}, false)
	text := r.String()
	if !strings.Contains(text, "failure details") || !strings.Contains(text, "update completed") {
		t.Fatal("the diagnostics are missing:", text)
	}
	if strings.Contains(text, "private conversation") || strings.Contains(text, "raw vendor data") {
		t.Fatal("the report carries what was said:", text)
	}
}
func TestBoundedLogsAndMissingSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	os.WriteFile(path, []byte(strings.Repeat("old row\n", 30000)+"final row\n"), 0600)
	var r Report
	r.File("tail", path)
	r.File("missing", path+".missing")
	if !strings.Contains(r.String(), "final row") || !strings.Contains(r.String(), "showing tail") || !strings.Contains(r.String(), "Unavailable:") {
		t.Fatal(r.String())
	}
	for range 30 {
		r.Add("large", strings.Repeat("x", FileLimit))
	}
	if r.Len() > Limit+100 || !strings.Contains(r.String(), "remaining sources omitted") {
		t.Fatal(r.Len())
	}
	var b Buffer
	b.Write([]byte(strings.Repeat("x", FileLimit*2)))
	b.Write([]byte("last"))
	if !strings.HasSuffix(b.String(), "last") || len(b.String()) > FileLimit+100 {
		t.Fatal("buffer bound")
	}
}

func TestTailRejectsSymlinks(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "secret"), []byte("not a log"), 0600)
	os.Symlink(filepath.Join(dir, "secret"), filepath.Join(dir, "log"))
	if _, _, err := Tail(filepath.Join(dir, "log"), 100); err == nil {
		t.Fatal("followed substituted log")
	}
}
