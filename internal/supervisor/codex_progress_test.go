package supervisor

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestCodexSilentResponseInterruptsOnceAndCompletionRecovers(t *testing.T) {
	t.Setenv("CXZ_CODEX_PROGRESS_TIMEOUT", "2m")
	s, input := displaySupervisor(t, "codex")
	s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"turn"}}}`))
	last := s.codex.progress.last
	s.consume([]byte(`{"method":"account/rateLimits/updated","params":{}}`))
	if !s.codex.progress.last.Equal(last) {
		t.Fatal("quota polling hid the response stall")
	}
	s.codex.checkProgress(last.Add(codexProgressTimeout - time.Second))
	if input.Len() != 0 {
		t.Fatal("interrupted before the response deadline")
	}
	now := last.Add(codexProgressTimeout)
	s.codex.checkProgress(now)
	if !bytes.Contains(input.Bytes(), []byte(`"method":"turn/interrupt"`)) || !bytes.Contains(input.Bytes(), []byte(`"turnId":"turn"`)) {
		t.Fatal("no interrupt for the stalled turn", input.String())
	}
	first := input.String()
	s.codex.checkProgress(now.Add(time.Second))
	if input.String() != first {
		t.Fatal("timeout duplicated the interrupt")
	}
	s.consume([]byte(`{"id":"cxz-progress-interrupt","result":{}}`))
	s.consume([]byte(`{"method":"turn/completed","params":{"turn":{"id":"turn","status":"interrupted"}}}`))
	s.codex.checkProgress(now.Add(time.Hour))
	if s.snap.State != "idle" || s.codex.turn != "" || !s.codex.progress.interrupt.IsZero() {
		t.Fatal("completed interrupt did not recover to idle")
	}
	if strings.Contains(input.String(), `"method":"turn/start"`) {
		t.Fatal("watchdog replayed user input")
	}
}

func TestCodexResponseTimeoutProtectsRunningItemsHumanInputAndStreaming(t *testing.T) {
	t.Setenv("CXZ_CODEX_PROGRESS_TIMEOUT", "2m")
	s, input := displaySupervisor(t, "codex")
	s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"turn"}}}`))
	s.consume([]byte(`{"method":"item/started","params":{"item":{"id":"tool","type":"mcpToolCall"}}}`))
	s.codex.checkProgress(time.Now().Add(time.Hour))
	if input.Len() != 0 {
		t.Fatal("interrupted a running tool")
	}
	s.consume([]byte(`{"method":"item/completed","params":{"item":{"id":"tool","type":"mcpToolCall"}}}`))
	s.codex.checkProgress(s.codex.progress.last.Add(time.Second))
	s.consume([]byte(`{"method":"item/reasoning/summaryTextDelta","params":{"delta":"progress"}}`))
	s.codex.checkProgress(s.codex.progress.last.Add(codexProgressTimeout - time.Second))
	if input.Len() != 0 {
		t.Fatal("interrupted streaming progress")
	}
	s.consume([]byte(`{"id":42,"method":"item/commandExecution/requestApproval","params":{"command":"long task"}}`))
	s.codex.checkProgress(time.Now().Add(time.Hour))
	if input.Len() != 0 || s.snap.State != "waiting_input" {
		t.Fatal("interrupted a human approval")
	}
}

func TestCodexUnacknowledgedResponseTimeoutBecomesResumableFailure(t *testing.T) {
	t.Setenv("CXZ_CODEX_PROGRESS_TIMEOUT", "2m")
	for _, rejected := range []bool{false, true} {
		t.Run(map[bool]string{false: "no completion", true: "rejected interrupt"}[rejected], func(t *testing.T) {
			s, input := displaySupervisor(t, "codex")
			s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"turn"}}}`))
			now := s.codex.progress.last.Add(codexProgressTimeout)
			s.codex.checkProgress(now)
			if rejected {
				s.consume([]byte(`{"id":"cxz-progress-interrupt","error":{"code":-32000,"message":"rejected"}}`))
			} else {
				s.codex.checkProgress(now.Add(codexInterruptGrace))
			}
			if s.snap.State != "failed" || s.snap.VendorID != "thread" {
				t.Fatal("unresponsive agent did not retain a resumable thread")
			}
			if strings.Contains(input.String(), `"method":"turn/start"`) {
				t.Fatal("failed watchdog replayed input")
			}
		})
	}
}

func TestCodexResponseDeadlineCanBeExtendedForSlowInference(t *testing.T) {
	t.Setenv("CXZ_CODEX_PROGRESS_TIMEOUT", "10m")
	s, input := displaySupervisor(t, "codex")
	s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"turn"}}}`))
	last := s.codex.progress.last
	s.codex.checkProgress(last.Add(5 * time.Minute))
	if input.Len() != 0 {
		t.Fatal("ignored the longer response budget")
	}
	s.codex.checkProgress(last.Add(10 * time.Minute))
	if !bytes.Contains(input.Bytes(), []byte(`"method":"turn/interrupt"`)) {
		t.Fatal("the extended response deadline was not enforced")
	}
}
