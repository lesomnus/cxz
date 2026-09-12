package agentview

import (
	"strings"
	"testing"
)

func TestProviderQuotaUnits(t *testing.T) {
	for _, tc := range []struct {
		provider, method, raw string
		remaining             float64
	}{
		{"codex", "account/rateLimits/updated", `{"rateLimits":{"primary":{"usedPercent":60,"windowDurationMins":300,"resetsAt":1900000000}}}`, 40},
		{"claude", "rate_limit_event", `{"rate_limit_info":{"rateLimitType":"five_hour","utilization":0.6,"resetsAt":1900000000}}`, 40},
		{"claude", "get_usage", `{"rate_limits":{"five_hour":{"utilization":60,"resets_at":"2030-01-01T00:00:00Z"}}}`, 40},
		{"claude", "rate_limit_event", `{"rate_limit_info":{"rateLimitType":"five_hour","utilization":1.1,"resetsAt":1900000000}}`, 0},
	} {
		v := Quota(tc.provider, tc.method, []byte(tc.raw))
		if len(v) != 1 || v[0].Remaining == nil || *v[0].Remaining != tc.remaining || v[0].Label != "5h" || v[0].Reset.IsZero() {
			t.Fatalf("%s %s: %+v", tc.provider, tc.method, v)
		}
	}
	for _, raw := range []string{`{"rate_limit_info":{"status":"allowed","utilization":null}}`, `{"rate_limit_info":{"status":"rejected"}}`, `{"rate_limit_info":{"utilization":-1}}`} {
		v := Quota("claude", "rate_limit_event", []byte(raw))
		if len(v) != 1 || v[0].Remaining != nil {
			t.Fatal("invented quota", v)
		}
	}
	if len(Quota("claude", "get_usage", []byte(`{"rate_limits_available":false,"rate_limits":null}`))) != 0 {
		t.Fatal("API quota invented")
	}
	if len(Quota("codex", "thread/tokenUsage/updated", []byte(`{"tokenUsage":{"last":{"totalTokens":123}}}`))) != 0 {
		t.Fatal("tokens mistaken for quota")
	}
}

func TestApprovalNormalizationRetainsFields(t *testing.T) {
	for _, tc := range []struct{ provider, method, title, raw string }{
		{"claude", "Bash", "Bash", `{"input":{"command":"echo 한국어"},"description":"Test","future":{"unknown":"retained"}}`},
		{"codex", "item/commandExecution/requestApproval", "Command", `{"params":{"command":"echo 한국어","reason":"Test","future":{"unknown":"retained"}}}`},
	} {
		v := ApprovalView(tc.provider, tc.method, []byte(tc.raw))
		if v.Title != tc.title || !strings.Contains(v.Detail, "echo 한국어") || !strings.Contains(v.Detail, `"unknown": "retained"`) {
			t.Fatal(v)
		}
	}
	v := ApprovalView("future", "unknown", []byte("unparsed data"))
	if v.Title != "unknown" || v.Detail != "unparsed data" {
		t.Fatal("unknown request lost")
	}
}

func TestCodexQuotaBuckets(t *testing.T) {
	raw := []byte(`{"rateLimits":{"limitId":"codex","primary":{"usedPercent":1}},"rateLimitsByLimitId":{"codex":{"primary":{"usedPercent":1,"windowDurationMins":300}},"other":{"limitName":"Other","primary":{"usedPercent":2,"windowDurationMins":60}}}}`)
	w := Quota("codex", "account/rateLimits/updated", raw)
	if len(w) != 2 || w[0].Key != "codex/primary" || w[1].Label != "Other/1h" {
		t.Fatal(w)
	}
	all, _ := QuotaReplacement("codex", "account/rateLimits/updated", raw)
	if !all {
		t.Fatal("full snapshot not recognized")
	}
	all, prefix := QuotaReplacement("codex", "account/rateLimits/updated", []byte(`{"rateLimits":{"limitId":"other","primary":{"usedPercent":3}}}`))
	if all || prefix != "other/" {
		t.Fatal("bucket notification clears unrelated limits")
	}
}
