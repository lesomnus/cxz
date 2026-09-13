package agentview

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ContextPercent adapts provider snapshots, never cumulative billing usage.
func ContextPercent(provider string, usage, limits []byte) (int, bool) {
	var used, window float64
	switch provider {
	case "codex":
		var v struct {
			TokenUsage struct {
				Last               struct{ TotalTokens *float64 }
				ModelContextWindow *float64
			}
		}
		if json.Unmarshal(usage, &v) != nil || v.TokenUsage.Last.TotalTokens == nil || v.TokenUsage.ModelContextWindow == nil {
			return 0, false
		}
		used, window = *v.TokenUsage.Last.TotalTokens, *v.TokenUsage.ModelContextWindow
		if used < 0 || window <= 0 {
			return 0, false
		}
		// Codex protocol TokenUsage::percent_of_context_window_remaining:
		// https://github.com/openai/codex/blob/main/codex-rs/protocol/src/protocol.rs
		if window <= 12000 {
			return 99, true
		}
		remaining := math.Round(math.Max(0, window-12000-math.Max(0, used-12000)) / (window - 12000) * 100)
		return int(math.Min(99, math.Max(0, 100-remaining))), true
	case "claude":
		var v struct {
			Model string
			Usage struct {
				Input *float64 `json:"input_tokens"`
				Read  float64  `json:"cache_read_input_tokens"`
				Write float64  `json:"cache_creation_input_tokens"`
			}
		}
		var l struct {
			ModelUsage map[string]struct {
				ContextWindow  float64
				CanonicalModel string
			}
		}
		if json.Unmarshal(usage, &v) != nil || json.Unmarshal(limits, &l) != nil || v.Usage.Input == nil {
			return 0, false
		}
		used = *v.Usage.Input + v.Usage.Read + v.Usage.Write
		window = l.ModelUsage[v.Model].ContextWindow
		if window == 0 && v.Model != "" {
			for _, model := range l.ModelUsage {
				if model.CanonicalModel == v.Model {
					if window != 0 && window != model.ContextWindow {
						return 0, false
					}
					window = model.ContextWindow
				}
			}
		}
		if *v.Usage.Input < 0 || v.Usage.Read < 0 || v.Usage.Write < 0 {
			return 0, false
		}
	default:
		return 0, false
	}
	if window <= 0 || used < 0 {
		return 0, false
	}
	return int(math.Min(99, math.Floor(used/window*100))), true
}

var contextTokensLine = regexp.MustCompile(`(?m)^\*\*Tokens:\*\*\s+[0-9]+(?:\.[0-9]+)?[kKmM]?\s*/\s*[0-9]+(?:\.[0-9]+)?[kKmM]?\s+\(([0-9]+(?:\.[0-9]+)?)%\)\s*$`)

// ClaudeContextReport accepts only the known native /context summary format.
// Callers must correlate it to their own /context request, not arbitrary prose.
func ClaudeContextReport(text string) (int, bool) {
	if !strings.Contains(text, "## Context Usage") || !strings.Contains(text, "**Model:**") {
		return 0, false
	}
	m := contextTokensLine.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	p, err := strconv.ParseFloat(m[1], 64)
	if err != nil || p < 0 || p > 100 {
		return 0, false
	}
	return min(99, int(p)), true
}

func HasContextLimits(raw []byte) bool {
	var v struct {
		ModelUsage map[string]struct{ ContextWindow float64 }
	}
	if json.Unmarshal(raw, &v) != nil {
		return false
	}
	for _, m := range v.ModelUsage {
		if m.ContextWindow > 0 {
			return true
		}
	}
	return false
}
