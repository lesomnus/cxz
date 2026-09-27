package agentview

import (
	"strings"
	"testing"
)

func TestClaudeContextNormalizedReport(t *testing.T) {
	raw := "## Context Usage\n\n**Model:** claude-opus-5[1m]\n**Tokens:** 109.7k / 1m (11%)\n\n### Estimated usage by category\n| Category | Tokens | Percentage |\n|---|---:|---:|\n| System prompt | 2,000 | 0.2% |\n| Messages | 107.7k | 10.8% |\n| Free space | 800k | 80% |\n\n### Tools\n| Tool | Tokens |\n| Bash | 1000 |"
	r := ParseClaudeContext(raw)
	if r.Model != "claude-opus-5[1m]" || r.Used == nil || *r.Used != 109700 || r.Window == nil || *r.Window != 1000000 {
		t.Fatal(r)
	}
	if len(r.Categories) != 3 || r.Categories[0].Tokens != 2000 {
		t.Fatal(r.Categories)
	}
	if p, ok := r.Utilization(); !ok || p != 10.97 {
		t.Fatal(p, ok)
	}
	r = ParseClaudeContext(strings.ReplaceAll(raw, "\n", "\r\n"))
	if r.Used == nil || *r.Used != 109700 {
		t.Fatal("CRLF response lost")
	}
	for _, text := range []string{"11%", "**Tokens:** 109.7k / 1m (11%)", "## Context Usage\n**Model:** x\n**Tokens:** unknown"} {
		r = ParseClaudeContext(text)
		if r.Used != nil || r.Window != nil || !strings.Contains(r.Note, "Raw") {
			t.Fatal("invented context", r)
		}
	}
}
func TestCodexContextUsesLastAndPreservesUnknown(t *testing.T) {
	r := ParseCodexContext([]byte(`{"tokenUsage":{"last":{"totalTokens":2000,"inputTokens":1800,"outputTokens":200,"cachedInputTokens":1600,"reasoningOutputTokens":100},"total":{"totalTokens":900000},"modelContextWindow":10000}}`))
	if r.Used == nil || *r.Used != 2000 || len(r.Metrics) != 4 || !r.Metrics[2].Subset || !r.Metrics[3].Subset {
		t.Fatal(r)
	}
	if p, ok := r.Utilization(); !ok || p != 20 {
		t.Fatal(p, ok)
	}
	for _, raw := range []string{`{`, `null`, `{}`, `{"tokenUsage":{"total":{"totalTokens":999999}}}`, `{"tokenUsage":{"last":{"totalTokens":-1},"modelContextWindow":0}}`} {
		r = ParseCodexContext([]byte(raw))
		if _, ok := r.Utilization(); ok || r.Used != nil {
			t.Fatal("unknown/invalid became usage", r)
		}
	}
	r = ParseCodexContext([]byte(`{"tokenUsage":{"last":{"totalTokens":0,"inputTokens":0},"modelContextWindow":10000}}`))
	if p, ok := r.Utilization(); !ok || p != 0 || len(r.Metrics) != 1 {
		t.Fatal("reported zero lost", r)
	}
}
