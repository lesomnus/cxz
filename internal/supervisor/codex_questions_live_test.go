package supervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
)

// Explicit opt-in: uses the existing Codex login and consumes subscription usage.
// Credentials are never opened, copied, printed, or included in fixtures.
func TestCodexAsyncQuestionLive(t *testing.T) {
	if os.Getenv("CXZ_CODEX_LIVE") != "1" {
		t.Skip("set CXZ_CODEX_LIVE=1 for a live authenticated probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s, _ := displaySupervisor(t, "codex")
	s.session.Workspace = t.TempDir()
	s.snap.VendorID = ""
	s.snap.State = "starting"
	cmd := exec.CommandContext(ctx, "codex", "app-server", "--listen", "stdio://")
	cmd.Dir = s.session.Workspace
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	s.stdin = in
	s.cmd = cmd
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 65536), 16*1024*1024)
		for sc.Scan() {
			s.consume(append([]byte{}, sc.Bytes()...))
		}
	}()
	defer func() { cancel(); in.Close(); cmd.Process.Kill(); <-done; cmd.Wait() }()
	if err := s.write(rpc("cxz-initialize", "initialize", map[string]any{"clientInfo": map[string]any{"name": "cxz-live-question", "version": "1"}, "capabilities": map[string]any{"experimentalApi": true}})); err != nil {
		t.Fatal(err)
	}
	wait := func(check func() bool) {
		t.Helper()
		for {
			s.mu.Lock()
			ok := check()
			s.mu.Unlock()
			if ok {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal("live protocol timed out")
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	wait(func() bool { return s.snap.State == "idle" })
	_, err = s.execute("send", core.Command{RunID: "run", ClientID: "live-ask", Text: "This is a UI protocol test. Do not read files, use shell, browse, delegate, or call any tool except request_user_input_async. Use that tool to ask exactly one question: Choose a theme, options Light and Dark. Do not merely print a question. Finish your turn after posting it. When you receive the answer, acknowledge only the chosen value."})
	if err != nil {
		t.Fatal(err)
	}
	wait(func() bool { return len(s.pending) > 0 && s.snap.State == "idle" })
	s.mu.Lock()
	var pending core.Event
	for _, p := range s.pending {
		pending = p
	}
	s.mu.Unlock()
	qs, err := agentview.Questions("codex", pending.Text, pending.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 1 {
		t.Fatal("expected one question")
	}
	encoded, err := agentview.EncodeQuestionAnswers(qs, [][]bool{make([]bool, len(qs[0].Options))}, []string{"Violet-731"})
	if err != nil {
		t.Fatal(err)
	}
	var selections map[string]core.AnswerSelection
	if err := json.Unmarshal([]byte(encoded), &selections); err != nil {
		t.Fatal(err)
	}
	_, err = s.execute("reply", core.Command{RunID: "run", ClientID: "live-answer", RequestID: pending.RequestID, Allow: true, Selections: selections})
	if err != nil {
		t.Fatal(err)
	}
	wait(func() bool {
		if s.snap.State != "idle" || len(s.pending) != 0 {
			return false
		}
		for _, e := range s.log.All() {
			if e.Kind == "assistant" && strings.Contains(e.Text, "Violet-731") {
				return true
			}
		}
		return false
	})
	t.Log("Verified native async question → common question model → structured Other answer → supervisor toolOutput → provider acknowledgement (Violet-731).")
}
