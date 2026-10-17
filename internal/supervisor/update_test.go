package supervisor

import (
	"bytes"
	"encoding/json"
	"fmt"
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
		if got := s.updateReason(now, true); got != "" {
			t.Fatal(got)
		}
		s.pending["q"] = core.Event{Text: "Question"}
		if got := s.updateReason(now, true); !strings.Contains(got, "pending") {
			t.Fatal(got)
		}
		delete(s.pending, "q")
		if s.updateReason(now, true) == "" {
			t.Fatal("idle window not reset after pending question")
		}
		if s.updateReason(now.Add(5*time.Minute), true) != "" {
			t.Fatal("continuous idle did not mature")
		}
		s.updateClients = map[string]updateClient{"client": {seen: now.Add(5 * time.Minute), busy: true}}
		if s.updateReason(now.Add(5*time.Minute), true) != "attached client busy" {
			t.Fatal("busy client accepted")
		}
		if s.updateReason(now.Add(6*time.Minute), true) == "" {
			t.Fatal("expired lease skipped fresh idle interval")
		}
	}
}

func TestUpdateBackgroundAndUnknownAreBlocked(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.lastActivity = time.Now().Add(-time.Hour)
	s.updateBackground.Tasks = map[string]agentview.BackgroundTask{"bg": {Active: true}}
	if s.updateReason(time.Now(), true) != "background tasks running" {
		t.Fatal("background accepted")
	}
	delete(s.updateBackground.Tasks, "bg")
	s.updateUnknown = true
	if s.updateReason(time.Now(), true) != "background telemetry unknown" {
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
		if !s.processesQuiet() {
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
		s.cmd = exec.Command("sh", "-c", `sh -c "read x" --state /cxz/state/data _mcp-bridge `+id+` cxz_memory; :`)
		s.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		s.cmd.Stdin = r
		t.Cleanup(func() { w.Close(); r.Close() })
		if err := s.cmd.Start(); err != nil {
			t.Fatal(err)
		}
		pid := s.cmd.Process.Pid
		t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL); _ = s.cmd.Wait() })
		children := fmt.Sprintf("/proc/%d/task/%d/children", pid, pid)
		for i := 0; ; i++ {
			b, err := os.ReadFile(children)
			if err != nil {
				t.Fatal(err)
			}
			if len(strings.Fields(string(b))) > 0 {
				break
			}
			if i == 100 {
				t.Fatal("fixture child never started")
			}
			time.Sleep(10 * time.Millisecond)
		}
		if got := s.processesQuiet(); got != bridge {
			t.Fatal("bridge", bridge, "quiet", got)
		}
	}
}

func TestActivityDoesNotFloodJournal(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	start := s.snap.LastSeq
	for i := 0; i < 10; i++ {
		b, _ := json.Marshal(core.Command{RunID: "run", ClientID: "ui", Busy: i%2 == 0})
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/activity", bytes.NewReader(b))
		s.serve(w, r)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	if s.snap.LastSeq != start || s.lastActivity.IsZero() {
		t.Fatal("activity journaling or clock broken")
	}
}
