package supervisor

import (
	"context"
	"github.com/lesomnus/cxz/internal/quotashare"
	"path/filepath"
	"testing"
	"time"
)

func TestQuotaPollingTimeoutAndRecovery(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			s, input := displaySupervisor(t, provider)
			poll := s.readClaudeQuota
			response := `{"type":"control_response","response":{"request_id":"cxz-quota","subtype":"success","response":{"rate_limits_available":false}}}`
			if provider == "codex" {
				poll = s.codex.readQuota
				response = `{"id":"cxz-quota","result":{"rateLimits":{}}}`
			}
			poll()
			first := input.Len()
			poll()
			if first == 0 || input.Len() != first || s.quotaRequested.IsZero() {
				t.Fatal("missing query or duplicate in-flight query")
			}
			s.quotaRequested = time.Now().Add(-time.Minute)
			poll()
			found := false
			for _, e := range s.log.All() {
				found = found || e.Kind == "usage_status" && e.Text == "timeout"
			}
			if !found || input.Len() <= first || s.snap.State != "idle" {
				t.Fatal("timeout/retry missing or changed conversation state")
			}
			s.consume([]byte(response))
			if !s.quotaRequested.IsZero() {
				t.Fatal("response did not complete request")
			}
			before := input.Len()
			poll()
			if input.Len() <= before {
				t.Fatal("polling failed to recover")
			}
		})
	}
}

func TestClaudeSharedQuotaAcrossSupervisors(t *testing.T) {
	root := t.TempDir()
	a, first := displaySupervisor(t, "claude")
	b, second := displaySupervisor(t, "claude")
	a.quotaRoot = root
	b.quotaRoot = root
	a.session.Account = "same"
	b.session.Account = "same"
	a.readClaudeQuota()
	b.readClaudeQuota()
	if first.Len() == 0 || second.Len() != 0 {
		t.Fatal("duplicate quota requests")
	}
	a.consume([]byte(`{"type":"control_response","response":{"request_id":"cxz-quota","subtype":"success","response":{"rate_limits":{"five_hour":{"utilization":20}}}}}`))
	b.readClaudeQuota()
	found := false
	for _, e := range b.log.All() {
		if e.Kind == "usage" && e.Text == "get_usage" {
			found = true
		}
	}
	if !found || second.Len() != 0 {
		t.Fatal("peer did not reuse account snapshot")
	}
}

func TestCodexLiveCommandOutput(t *testing.T) {
	s, _ := displaySupervisor(t, "codex")
	s.consume([]byte(`{"method":"item/commandExecution/outputDelta","params":{"itemId":"tool","delta":"hello\n","threadId":"thread","turnId":"turn"}}`))
	found := false
	for _, e := range s.log.All() {
		if e.Kind == "tool_output" && e.RequestID == "tool" && e.Text == "hello\n" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing output event")
	}
}

func TestRemoteQuotaDoesNotBlockAgentEvents(t *testing.T) {
	root := t.TempDir()
	socket := filepath.Join(root, "quota.sock")
	gate := make(chan struct{})
	entered := make(chan struct{})
	srv, err := quotashare.Start(root, socket, func(_ context.Context, _ quotashare.Request, _ string) bool { close(entered); <-gate; return true })
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	s, input := displaySupervisor(t, "claude")
	s.quotaRoot = root
	s.quotaSocket = socket
	s.quotaToken = "token"
	s.session.Account = "shared"
	s.mu.Lock()
	s.readClaudeQuota()
	s.mu.Unlock()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(gate)
		s.quotaWorkers.Wait()
		t.Fatal("quota coordinator was not reached")
	}
	processed := make(chan struct{})
	go func() {
		s.consume([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"still responsive"}]}}`))
		close(processed)
	}()
	select {
	case <-processed:
	case <-time.After(time.Second):
		close(gate)
		s.quotaWorkers.Wait()
		t.Fatal("quota blocked provider output")
	}
	close(gate)
	s.quotaWorkers.Wait()
	if input.Len() == 0 {
		t.Fatal("claimed quota was not requested")
	}
}
