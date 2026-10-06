package supervisor

import (
	"encoding/json"
	"math"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

type responseTracking struct {
	started     time.Time
	last, final uint64
	phase       string
	tools       int
	metrics     map[string]float64
}

func metric(out map[string]float64, key string, value any, integer bool) {
	n, ok := value.(float64)
	if ok && n >= 0 && !math.IsInf(n, 0) && !math.IsNaN(n) && (!integer || n <= 9007199254740991 && math.Trunc(n) == n) {
		out[key] = n
	}
}
func object(v any) map[string]any { m, _ := v.(map[string]any); return m }

func (s *Supervisor) captureResponseUsage(raw []byte) {
	var p map[string]any
	if json.Unmarshal(raw, &p) != nil || s.responseContext.TurnID == "" || p["turnId"] != s.responseContext.TurnID {
		return
	}
	if id, ok := p["threadId"].(string); ok && id != s.snap.VendorID {
		return
	}
	last := object(object(p["tokenUsage"])["last"])
	out := map[string]float64{}
	for native, common := range map[string]string{"inputTokens": "input_tokens", "outputTokens": "output_tokens", "cachedInputTokens": "cache_read_tokens", "reasoningOutputTokens": "reasoning_tokens", "totalTokens": "total_tokens"} {
		metric(out, common, last[native], true)
	}
	s.responseTracking.metrics = out
}

func (s *Supervisor) completeResponse(status string, raw []byte) core.ResponseMetadata {
	metadata := s.responseContext
	if status != "completed" {
		return metadata
	}
	t := s.responseTracking
	seq, source := t.final, "phase"
	if seq == 0 && t.phase == "" {
		seq, source = t.last, "turn_end"
	}
	if seq == 0 {
		return metadata
	}
	c := core.ResponseCompletion{ResponseSeq: seq, FinalSource: source, Metrics: map[string]float64{}}
	var p map[string]any
	_ = json.Unmarshal(raw, &p)
	if s.codex != nil {
		if id, ok := object(p["turn"])["id"].(string); ok && id != "" && id != metadata.TurnID {
			return metadata
		}
	}
	var duration any
	if s.codex == nil {
		duration = p["duration_ms"]
		c.TokenScope = "turn"
		usage := object(p["usage"])
		for native, common := range map[string]string{"input_tokens": "input_tokens", "output_tokens": "output_tokens", "cache_read_input_tokens": "cache_read_tokens", "cache_creation_input_tokens": "cache_write_tokens"} {
			metric(c.Metrics, common, usage[native], true)
		}
		metric(c.Metrics, "cost_usd", p["total_cost_usd"], false)
		metric(c.Metrics, "api_duration_ms", p["duration_api_ms"], true)
		metric(c.Metrics, "api_turns", p["num_turns"], true)
	} else {
		duration = object(p["turn"])["durationMs"]
		c.TokenScope = "last_call"
		for k, v := range t.metrics {
			c.Metrics[k] = v
		}
	}
	valid := map[string]float64{}
	metric(valid, "duration", duration, true)
	if n, ok := valid["duration"]; ok {
		ms := int64(n)
		c.DurationMS, c.DurationSource = &ms, "provider"
	} else if !t.started.IsZero() {
		ms := max(0, time.Since(t.started).Milliseconds())
		c.DurationMS, c.DurationSource = &ms, "cxz"
	}
	if !t.started.IsZero() {
		c.Metrics["tool_calls"] = float64(t.tools)
	}
	b, _ := json.Marshal(c)
	metadata.CompletionJSON = string(b)
	return metadata
}
