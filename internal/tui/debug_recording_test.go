package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func TestControlWordMovementAndRecordingPrivacy(t *testing.T) {
	m := conversationModel()
	m.ctx = WithRecordingDirectory(context.Background(), t.TempDir())
	m.input.SetValue("private-token alpha beta")
	m.input.CursorEnd()
	m.toggleRecording()
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlLeft})
	pos := m.input.LineInfo().StartColumn + m.input.LineInfo().ColumnOffset
	if pos >= len([]rune(m.input.Value())) {
		t.Fatal("Ctrl+Left did not move by word")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if m.input.LineInfo().StartColumn+m.input.LineInfo().ColumnOffset <= pos {
		t.Fatal("Ctrl+Right did not move")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("PRIVATE-TEXT")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("SECRET-PASTE"), Paste: true})
	m.notice = "credential from server: SECRET-NOTICE"
	view := m.View()
	if !strings.Contains(ansi.Strip(view), "REC") {
		t.Fatal("missing visible recording status")
	}
	m.Update(m.toggleRecording()())
	if m.lastRecording == "" || m.debugRecorder.Active() {
		t.Fatal("recording not saved")
	}
	b, err := os.ReadFile(m.lastRecording)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-token", "PRIVATE-TEXT", "SECRET-PASTE", "SECRET-NOTICE"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("recorded private content", secret)
		}
	}
	if !strings.Contains(string(b), "word_backward") || !strings.Contains(string(b), "ctrl+left") || !strings.Contains(string(b), "render") {
		t.Fatal("missing debug detail", string(b))
	}
	st, err := os.Stat(m.lastRecording)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("recording permissions", st, err)
	}
}
func TestRecordingSaveFailureRetryAndExit(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "recordings")
	if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	m := conversationModel()
	m.ctx = WithRecordingDirectory(context.Background(), blocked)
	m.toggleRecording()
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m.Update(m.toggleRecording()())
	if m.recordingPending == nil || m.recordingSaving || m.lastRecording != "" {
		t.Fatal("failed save discarded recording")
	}
	os.Remove(blocked)
	m.Update(m.toggleRecording()())
	if m.recordingPending != nil || m.lastRecording == "" {
		t.Fatal("retry failed")
	}
	previous := m.lastRecording
	m.toggleRecording()
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	path, err := m.finishRecording()
	if err != nil || path == previous || path == "" {
		t.Fatal("exit failed to save", path, err)
	}
}
func TestDebugRecorderBoundedAndConcurrent(t *testing.T) {
	r := &debugRecorder{}
	r.Start()
	for i := 0; i < debugEventLimit+5; i++ {
		r.Add(debugEvent{Kind: "test", Count: i})
	}
	a := r.Stop()
	if len(a.Events) != debugEventLimit || a.Dropped != 5 || a.Events[0].Count != 5 || a.Events[len(a.Events)-1].Count != debugEventLimit+4 {
		t.Fatal("ring lost ordering")
	}
	r.Start()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				r.Add(debugEvent{Kind: "input"})
			}
		}()
	}
	wg.Wait()
	if len(r.Stop().Events) != 400 {
		t.Fatal("lost concurrent events")
	}
}
func TestRecordingButtonWithoutDockerAndSingleSaveOnExit(t *testing.T) {
	m := conversationModel()
	m.ctx = WithRecordingDirectory(context.Background(), t.TempDir())
	m.settingsPage = &settingsPage{loading: true, selected: 3}
	m.activateSetting()
	if !m.debugRecorder.Active() {
		t.Fatal("record button requires Docker")
	}
	cmd := m.activateSetting()
	if cmd == nil || !m.recordingSaving {
		t.Fatal("button did not stop")
	}
	path, err := m.finishRecording()
	if err != nil || path == "" {
		t.Fatal(path, err)
	}
	m.Update(cmd()) // Late tea result must not write another archive.
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("duplicate save")
	}
	b, _ := os.ReadFile(path)
	var header debugArchive
	if err = json.Unmarshal([]byte(strings.Split(string(b), "\n")[0]), &header); err != nil || header.Format != 1 {
		t.Fatal("invalid archive", err)
	}
}

// A slow transcript rebuild and a large one look the same with only a row
// count: the rows are what survived the window, while the cost is every loaded
// event. Record both, and the turn count, since the window cannot trim below a
// single turn however large it grows.
func TestTranscriptRenderRecordsWhatItWalked(t *testing.T) {
	// The walk is proportional to the events, so it is gated on a live recording.
	if (*debugRecorder)(nil).recording() || new(debugRecorder).recording() {
		t.Fatal("an idle recorder would make render pay to measure itself")
	}
	m := conversationModel()
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "run", Kind: "input", Text: "hello"},
		{Seq: 2, RunId: "run", Kind: "tool_output", RequestId: "t", Text: "0123456789"},
		{Seq: 3, RunId: "run", Kind: "assistant", Text: "hi"},
	}
	m.debugRecorder = &debugRecorder{}
	m.debugRecorder.Start()
	m.limitHistory("s", false, 0)
	m.render()
	var found *debugEvent
	for _, e := range m.debugRecorder.Stop().Events {
		if e.Kind == "transcript_render" {
			found = &e
		}
	}
	if found == nil {
		t.Fatal("no transcript_render recorded")
	}
	if found.Events != 3 {
		t.Fatalf("walked events not recorded: %d", found.Events)
	}
	if found.Bytes != len("hello")+len("0123456789")+len("hi") {
		t.Fatalf("walked bytes not recorded: %d", found.Bytes)
	}
	if found.Turns == 0 {
		t.Fatal("window turns not recorded")
	}
	if found.Count == 0 {
		t.Fatal("row count was lost")
	}
}
