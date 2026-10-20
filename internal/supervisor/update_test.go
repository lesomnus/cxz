package supervisor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
)

func TestUpdateIdleGate(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		s, _ := displaySupervisor(t, provider)
		now := time.Now()
		s.lastActivity = now.Add(-6 * time.Minute)
		if got := s.updateReason(now, ""); got != "" {
			t.Fatal(got)
		}
		s.pending["q"] = core.Event{Text: "Question"}
		if got := s.updateReason(now, ""); !strings.Contains(got, "pending") {
			t.Fatal(got)
		}
		delete(s.pending, "q")
		if s.updateReason(now, "") == "" {
			t.Fatal("idle window not reset after pending question")
		}
		if s.updateReason(now.Add(5*time.Minute), "") != "" {
			t.Fatal("continuous idle did not mature")
		}
		s.updateClients = map[string]updateClient{"client": {seen: now.Add(5 * time.Minute), busy: true}}
		if s.updateReason(now.Add(5*time.Minute), "") != "attached client busy" {
			t.Fatal("busy client accepted")
		}
		// The lease expires and the session is as quiet as it was before anyone
		// attached. It used to serve another five minutes here, which a client
		// that reattaches every few minutes made permanent. See cxz#249.
		if got := s.updateReason(now.Add(6*time.Minute), ""); got != "" {
			t.Fatal("an expired lease served a fresh idle interval:", got)
		}
	}
}

func TestUpdateBackgroundAndUnknownAreBlocked(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.lastActivity = time.Now().Add(-time.Hour)
	s.updateBackground.Tasks = map[string]agentview.BackgroundTask{"bg": {Active: true}}
	if s.updateReason(time.Now(), "") != "background tasks running" {
		t.Fatal("background accepted")
	}
	delete(s.updateBackground.Tasks, "bg")
	s.updateUnknown = true
	if s.updateReason(time.Now(), "") != "background telemetry unknown" {
		t.Fatal("unknown accepted")
	}
}

func TestUpdateStopFencesNewInput(t *testing.T) {
	for _, inputFirst := range []bool{false, true} {
		s, _ := displaySupervisor(t, "claude")
		s.cmd = exec.Command("sleep", "20")
		s.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := s.cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.cmd.Process.Kill(); _ = s.cmd.Wait() })
		s.lastActivity = time.Now().Add(-6 * time.Minute)
		if s.busyProcess() != "" {
			t.Fatal("fixture process unexpectedly busy")
		}
		if inputFirst {
			if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "send", Text: "hello"}); err != nil {
				t.Fatal(err)
			}
		}
		_, err := s.execute("update-stop", core.Command{RunID: "run", ClientID: "update"})
		if inputFirst {
			if err == nil || s.stopping {
				t.Fatal("update interrupted new input")
			}
		} else {
			if err != nil || !s.stopping {
				t.Fatal("eligible update did not stop", err)
			}
			if _, err = s.execute("send", core.Command{RunID: "run", ClientID: "late", Text: "hello"}); err == nil {
				t.Fatal("input crossed restart fence")
			}
		}
	}
}

func TestProcessQuietIgnoresOwnBridgesAndExitedChildren(t *testing.T) {
	for _, c := range []struct {
		state string
		args  []string
		quiet bool
	}{
		{"S", []string{"/cxz/cxz", "--state", "/cxz/state/data", "_mcp-bridge", "s", "cxz_memory"}, true},
		{"S", []string{"/cxz/cxz", "--state", "/cxz/state/data", "_mcp-bridge", "other", "cxz_memory"}, false},
		{"Z", []string{"/bin/zsh", "-c", "go build ./..."}, true},
		{"S", []string{"/bin/zsh", "-c", "go build ./..."}, false},
		{"S", nil, false},
		// Codex keeps this one for the length of the session, exactly as the
		// bridges are kept. It was found blocking every update of an
		// installation for as long as one idle session existed. See cxz#235.
		{"S", []string{"/cxz/tools/codex/0.163.0/x86_64-unknown-linux-musl/bin/codex-code-mode-host"}, true},
		// Named rather than matched anywhere in the arguments: a process of the
		// person's own that merely mentions it is still work.
		{"S", []string{"/bin/zsh", "-c", "codex-code-mode-host --help"}, false},
	} {
		var cmdline []byte
		for _, a := range c.args {
			cmdline = append(append(cmdline, a...), 0)
		}
		if got := processQuiet(c.state, cmdline, "s"); got != c.quiet {
			t.Fatal(c.state, c.args, got)
		}
	}
}

// A session whose agent still has cxz's own MCP bridges attached is quiet: they
// run for as long as the session does, so counting them blocks every update.
func TestProcessesQuietWithLiveBridge(t *testing.T) {
	for _, bridge := range []bool{true, false} {
		s, _ := displaySupervisor(t, "claude")
		id := "other"
		if bridge {
			id = s.session.ID
		}
		// A child that blocks on a builtin read: one process, no children of
		// its own, with the argv a bridge is spawned with.
		s.cmd = exec.Command("sh", "-c", `sh -c "printf r; read x" --state /cxz/state/data _mcp-bridge `+id+` cxz_memory; :`)
		s.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		s.cmd.Stdin = r
		t.Cleanup(func() { w.Close(); r.Close() })
		ready, signal, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		s.cmd.Stdout = signal
		t.Cleanup(func() { ready.Close(); signal.Close() })
		if err := s.cmd.Start(); err != nil {
			t.Fatal(err)
		}
		pid := s.cmd.Process.Pid
		t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL); _ = s.cmd.Wait() })
		_ = signal.Close()
		if err := ready.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
			t.Fatal(err)
		}
		// A forked child exists before exec replaces its inherited command line.
		// Wait for the fixture shell itself, not merely its PID, to be ready.
		if _, err := io.ReadFull(ready, make([]byte, 1)); err != nil {
			t.Fatal("fixture child never became ready:", err)
		}

		busy := s.busyProcess()
		if got := busy == ""; got != bridge {
			t.Fatal("bridge", bridge, "quiet", got)
		}
		// A refusal has to say which process it is waiting for. Without that, a
		// session that will never be quiet again reads exactly like one that is
		// mid-turn, and telling them apart means reading /proc on the
		// installation by hand. See cxz#235.
		if !bridge && !strings.Contains(busy, "sh (pid ") {
			t.Fatal("the refusal does not name what is running:", busy)
		}
	}
}

// Reporting activity is how a client says it is there, and it neither writes to
// the journal nor moves the idle clock -- not even the first report from a
// client ID the supervisor has never seen. The web mints an ID per stream, so a
// tab that reconnects is a new client every few minutes; counting that as an
// interaction is what kept a session at "waiting for 5 minutes of continuous
// idle" indefinitely. See cxz#249.
func TestActivityNeitherFloodsTheJournalNorMovesTheClock(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	start := s.snap.LastSeq
	quiet := time.Now().Add(-time.Hour)
	s.lastActivity = quiet
	for i := 0; i < 10; i++ {
		b, _ := json.Marshal(core.Command{RunID: "run", ClientID: fmt.Sprintf("tab-%d", i), Busy: i%2 == 0})
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/activity", bytes.NewReader(b))
		s.serve(w, r)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	if s.snap.LastSeq != start {
		t.Fatal("activity was journaled")
	}
	if !s.lastActivity.Equal(quiet) {
		t.Fatal("ten clients attaching moved the idle clock")
	}
}

// A turn ending ends what the turn made uncertain. Neither of these was ever
// cleared, so one ambiguous frame or one tool result that never arrived blocked
// every update of the installation for as long as the session lived. The frame
// that made it concrete is Claude's own subagent progress report. See cxz#244.
func TestATurnEndingClearsWhatTheTurnMadeUncertain(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.snap.State = "idle"
	s.lastActivity = time.Now().Add(-time.Hour)

	// Subagent progress: a system frame whose subtype has "task" in it.
	s.observeUpdateRaw([]byte(`{"type":"system","subtype":"task_progress","task_id":"a4","subagent_type":"Explore","description":"Running something"}`))
	if !s.updateUnknown {
		t.Fatal("subagent progress no longer reads as uncertain; this test is watching the wrong thing")
	}
	if got := s.updateReason(time.Now(), ""); got != "background telemetry unknown" {
		t.Fatal("expected the session to refuse while the turn runs, got", got)
	}

	// A tool call whose result never arrives is the same shape.
	s.observeUpdateEvent("tool", "Bash", "t1")
	if got := s.updateReason(time.Now(), ""); got == "" {
		t.Fatal("a session with an unfinished tool call was ready")
	}

	s.observeUpdateEvent("turn_end", "completed", "")
	if s.updateUnknown || len(s.updateTools) != 0 {
		t.Fatal("the turn ended and the uncertainty did not", s.updateUnknown, s.updateTools)
	}
	// The idle clock is what remains, and it moved because the turn ended --
	// which is the refusal that resolves on its own.
	if got := s.updateReason(time.Now(), ""); got != "waiting for 5 minutes of continuous idle" {
		t.Fatal("after the turn the session refuses for another reason:", got)
	}
	if got := s.updateReason(time.Now().Add(6*time.Minute), ""); got != "" {
		t.Fatal("the session never becomes ready:", got)
	}
}
