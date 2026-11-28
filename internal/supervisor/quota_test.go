package supervisor

import (
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
			// An unanswered request is noticed at the next scheduled poll, not
			// in between: how often to ask is quotaPolledAt's to say and how
			// long to wait for an answer is quotaRequested's.
			s.quotaRequested = time.Now().Add(-time.Minute)
			s.quotaPolledAt = time.Now().Add(-time.Minute)
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
			s.quotaPolledAt = time.Now().Add(-time.Minute)
			poll()
			if input.Len() <= before {
				t.Fatal("polling failed to recover")
			}
		})
	}
}

// Two sessions under one account alias can hold credentials for different
// provider accounts, because Claude's only backend logs in per session profile.
// A shared poll therefore showed one session the other's limit; see cxz#400.
// Nothing about the session says which case this is -- and the backend string
// is not asked either, so a session whose manifest predates that field still
// polls for itself rather than falling back to sharing.
func TestClaudeQuotaUsesSessionOAuthLogin(t *testing.T) {
	root := t.TempDir()
	a, first := displaySupervisor(t, "claude")
	b, second := displaySupervisor(t, "claude")
	for _, s := range []*Supervisor{a, b} {
		s.quotaRoot = root
		s.session.Account = "work"
		s.session.AuthBackend = ""
	}
	a.readClaudeQuota()
	if first.Len() == 0 {
		t.Fatal("first session did not poll its Claude process")
	}
	a.consume([]byte(`{"type":"control_response","response":{"request_id":"cxz-quota","subtype":"success","response":{"rate_limits":{"seven_day":{"utilization":1}}}}}`))
	b.readClaudeQuota()
	if second.Len() == 0 || recorded(b, "usage", "get_usage") != 0 {
		t.Fatal("second session did not poll its own login independently")
	}
	b.consume([]byte(`{"type":"control_response","response":{"request_id":"cxz-quota","subtype":"success","response":{"rate_limits":{"seven_day":{"utilization":99}}}}}`))
	before := first.Len()
	a.readClaudeQuota()
	if first.Len() != before || recorded(a, "usage", "get_usage") != 1 {
		t.Fatal("first session polled early or picked up the second session's quota")
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
