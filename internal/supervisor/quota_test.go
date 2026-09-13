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
