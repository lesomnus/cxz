package usagemetrics_test

import (
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/usagemetrics"
)

// These were checked through the history-based statistics path, which is gone;
// the telemetry publisher normalizes committed events with the same state.
func TestStateNormalizesCodexAndCumulativeCost(t *testing.T) {
	s := usagemetrics.State{}
	var total usagemetrics.Values
	for i, e := range []core.Event{
		{Kind: "turn_end", RunID: "run", Payload: []byte(`{"total_cost_usd":2}`)},
		{Kind: "input", RunID: "run", Text: "한글"},
		{Kind: "usage", RunID: "run", Text: "thread/tokenUsage/updated", Payload: []byte(`{"tokenUsage":{"last":{"inputTokens":100,"cachedInputTokens":40,"outputTokens":10}}}`)},
		{Kind: "turn_end", RunID: "run", Payload: []byte(`{"total_cost_usd":3}`)},
	} {
		v := s.Event(e)
		// The first report precedes the period being summed: it only sets the
		// cumulative baseline the later report is differenced against.
		if i > 0 {
			total.Add(v)
		}
	}
	if total.ConversationBytes != 6 || total.InputTokens != 60 || total.CacheReadTokens != 40 || total.OutputTokens != 10 || total.ReportedCostUSD != 1 || total.TokenReportedTurns != 1 {
		t.Fatalf("%+v", total)
	}
}

func TestStateMissingAndRunIsolation(t *testing.T) {
	s := usagemetrics.State{}
	s.Event(core.Event{Kind: "usage", RunID: "old", Text: "thread/tokenUsage/updated", Payload: []byte(`{"tokenUsage":{"last":{"inputTokens":500}}}`)})
	got := s.Event(core.Event{Kind: "turn_end", RunID: "new"})
	if got.TokenReportedTurns != 0 || got.CostReportedTurns != 0 {
		t.Fatalf("%+v", got)
	}
	got = s.Event(core.Event{Kind: "turn_end", RunID: "new", Payload: []byte(`{"usage":{"input_tokens":20,"output_tokens":3,"cache_read_input_tokens":50,"cache_creation_input_tokens":10},"costUSD":0}`)})
	if got.InputTokens != 20 || got.CacheReadTokens != 50 || got.CostReportedTurns != 1 || got.ReportedCostUSD != 0 {
		t.Fatalf("%+v", got)
	}
}
