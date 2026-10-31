// Package usagemetrics normalizes committed provider events without retaining
// prompts or responses. Both retained-history statistics and live OTLP use it.
package usagemetrics

import (
	"encoding/json"
	"math"

	"github.com/lesomnus/cxz/internal/core"
)

type Values struct {
	Events, InputMessages, AssistantEvents, ConversationBytes, EventContentBytes uint64
	FinishedTurns, InputTokens, OutputTokens, CacheReadTokens, CacheWriteTokens  uint64
	TokenReportedTurns, CostReportedTurns                                        uint64
	ReportedCostUSD                                                              float64
}

func (v *Values) Add(o Values) {
	v.Events += o.Events
	v.InputMessages += o.InputMessages
	v.AssistantEvents += o.AssistantEvents
	v.ConversationBytes += o.ConversationBytes
	v.EventContentBytes += o.EventContentBytes
	v.FinishedTurns += o.FinishedTurns
	v.InputTokens += o.InputTokens
	v.OutputTokens += o.OutputTokens
	v.CacheReadTokens += o.CacheReadTokens
	v.CacheWriteTokens += o.CacheWriteTokens
	v.TokenReportedTurns += o.TokenReportedTurns
	v.CostReportedTurns += o.CostReportedTurns
	v.ReportedCostUSD += o.ReportedCostUSD
}

type State struct {
	pending map[string]any
	run     string
	cost    map[string]float64
	known   map[string]bool
	Gap     bool
}

func object(m map[string]any, key string) map[string]any { v, _ := m[key].(map[string]any); return v }
func number(m map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		v, ok := m[key].(float64)
		if ok && v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) && v < math.MaxInt64 {
			return v, true
		}
	}
	return 0, false
}

func (s *State) Event(e core.Event) Values {
	v := Values{Events: 1, EventContentBytes: uint64(len(e.Text) + len(e.Payload))}
	if e.Kind == "input" {
		v.InputMessages = 1
		s.pending = nil
	}
	if e.Kind == "assistant" {
		v.AssistantEvents = 1
	}
	if e.Kind == "input" || e.Kind == "assistant" {
		v.ConversationBytes = uint64(len(e.Text))
	}
	// Avoid unmarshalling large tool/response bodies when only their size matters.
	if e.Kind != "usage" && e.Kind != "turn_end" {
		return v
	}
	var root map[string]any
	_ = json.Unmarshal(e.Payload, &root)
	if e.Kind == "usage" && e.Text == "thread/tokenUsage/updated" {
		s.pending = object(object(root, "tokenUsage"), "last")
		s.run = e.RunID
	}
	if e.Kind != "turn_end" {
		return v
	}
	v.FinishedTurns = 1
	u := object(root, "usage")
	if len(u) == 0 {
		u = object(object(root, "turn"), "usage")
	}
	if len(u) == 0 && s.run == e.RunID {
		u = s.pending
	}
	s.pending = nil
	for _, field := range []struct {
		keys []string
		dst  *uint64
	}{
		{[]string{"input_tokens", "inputTokens"}, &v.InputTokens},
		{[]string{"output_tokens", "outputTokens"}, &v.OutputTokens},
		{[]string{"cache_read_input_tokens", "cachedInputTokens"}, &v.CacheReadTokens},
		{[]string{"cache_creation_input_tokens"}, &v.CacheWriteTokens},
	} {
		if n, ok := number(u, field.keys...); ok {
			*field.dst = uint64(n)
			v.TokenReportedTurns = 1
		}
	}
	if _, codex := u["inputTokens"]; codex {
		v.InputTokens -= min(v.InputTokens, v.CacheReadTokens)
	}
	if n, ok := number(root, "total_cost_usd"); ok {
		if s.cost == nil {
			s.cost = map[string]float64{}
			s.known = map[string]bool{}
		}
		if s.known[e.RunID] || !s.Gap {
			v.ReportedCostUSD = n - s.cost[e.RunID]
			if v.ReportedCostUSD < 0 {
				v.ReportedCostUSD = n
			}
			v.CostReportedTurns = 1
		}
		s.cost[e.RunID] = n
		s.known[e.RunID] = true
	} else if n, ok := number(root, "costUSD", "costUsd"); ok {
		v.ReportedCostUSD = n
		v.CostReportedTurns = 1
	}
	return v
}
