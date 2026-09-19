package logview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func TestJournalOmitsConversationAndUncommittedTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	events := []core.Event{{Kind: "assistant", Text: "private conversation", Raw: []byte("raw vendor data")}, {Kind: "update", Text: "update completed"}, {Kind: "diagnostic", Text: "failure details"}}
	b, _ := json.Marshal(events)
	b = append(b, '\n')
	b = append(b, []byte(`{"kind":"diagnostic","text":"uncommitted"}`)...)
	os.WriteFile(path, b, 0600)
	var r Report
	r.Journal("journal", path)
	text := r.String()
	if !strings.Contains(text, "failure details") || !strings.Contains(text, "update completed") || strings.Contains(text, "private conversation") || strings.Contains(text, "raw vendor data") || strings.Contains(text, "uncommitted") {
		t.Fatal(text)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(b) {
		t.Fatal("reading repaired the journal")
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
