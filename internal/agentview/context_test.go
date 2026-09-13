package agentview

import "testing"

func TestContextPercent(t *testing.T) {
	for _, tc := range []struct {
		provider, usage, limits string
		want                    int
		ok                      bool
	}{
		{"codex", `{"tokenUsage":{"last":{"totalTokens":47000},"total":{"totalTokens":999999},"modelContextWindow":112000}}`, "", 35, true},
		{"codex", `{"tokenUsage":{"last":{"totalTokens":200000},"modelContextWindow":112000}}`, "", 99, true},
		{"codex", `{"tokenUsage":{"last":{"totalTokens":0},"modelContextWindow":112000}}`, "", 0, true},
		{"codex", `{"tokenUsage":{"last":{}}}`, "", 0, false},
		{"claude", `{"model":"x","usage":{"input_tokens":1000,"cache_read_input_tokens":33000,"cache_creation_input_tokens":1000}}`, `{"modelUsage":{"x[1m]":{"canonicalModel":"x","contextWindow":100000,"inputTokens":999999}}}`, 35, true},
		{"claude", `{"model":"x","usage":{"input_tokens":1000}}`, `{"modelUsage":{"y":{"contextWindow":100000}}}`, 0, false},
	} {
		got, ok := ContextPercent(tc.provider, []byte(tc.usage), []byte(tc.limits))
		if got != tc.want || ok != tc.ok {
			t.Fatalf("%+v: %d %v", tc, got, ok)
		}
	}
}
