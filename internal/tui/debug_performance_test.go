package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type slowTerminal struct{ entered, release chan struct{} }

func (s *slowTerminal) Write(p []byte) (int, error) {
	close(s.entered)
	<-s.release
	return len(p), nil
}

func TestBlockedTerminalDoesNotBlockCursorPosition(t *testing.T) {
	sink := &slowTerminal{make(chan struct{}), make(chan struct{})}
	r := &debugRecorder{}
	r.Start()
	w := &cursorWriter{out: sink, recorder: r}
	done := make(chan struct{})
	go func() { w.Write([]byte("PRIVATE SCREEN CONTENT")); close(done) }()
	<-sink.entered
	positioned := make(chan struct{})
	go func() { w.position(3, 4, true); close(positioned) }()
	select {
	case <-positioned:
	case <-time.After(time.Second):
		close(sink.release)
		<-done
		t.Fatal("terminal write blocked UI cursor update")
	}
	close(sink.release)
	<-done
	a := r.Stop()
	if len(a.Events) != 1 || a.Events[0].Kind != "terminal_write" || a.Events[0].Count != len("PRIVATE SCREEN CONTENT") || a.Events[0].Duration < a.Events[0].IO {
		t.Fatal(a.Events)
	}
	b, _ := json.Marshal(a.Events)
	if strings.Contains(string(b), "PRIVATE") {
		t.Fatal("screen contents leaked")
	}
}

func TestPerformanceRecordingSamplesAndRejectsOldTicks(t *testing.T) {
	m := conversationModel()
	cmd := m.toggleRecording()
	tick, ok := cmd().(performanceTick)
	if !ok || len(tick.metrics) != 4 {
		t.Fatal("missing runtime samples", tick)
	}
	tick.ready = time.Now().Add(-100 * time.Millisecond)
	if next := m.performanceUpdate(tick); next == nil {
		t.Fatal("sampling stopped")
	}
	a := m.debugRecorder.Stop()
	if len(a.Events) != 1 || a.Events[0].Wait < 100000 || a.Events[0].Metrics["/sched/goroutines:goroutines"] == 0 {
		t.Fatal(a.Events)
	}
	m.debugRecorder.Start()
	if m.performanceUpdate(tick) != nil {
		t.Fatal("old tick started duplicate sampler")
	}
	if len(m.debugRecorder.Stop().Events) != 0 {
		t.Fatal("old tick entered new recording")
	}
}
