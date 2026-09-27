package agentview

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ContextReport is a provider snapshot, not cumulative billing usage. Nil counts
// mean unreported, and cached/reasoning metrics are subsets, not extra tokens.
type ContextReport struct {
	Provider, Model, Basis string
	Used, Window           *float64
	Categories             []ContextMetric
	Metrics                []ContextMetric
	Note                   string
}
type ContextMetric struct {
	Name   string
	Tokens float64
	Subset bool
}

func (r ContextReport) Utilization() (float64, bool) {
	if r.Used == nil || r.Window == nil || *r.Window <= 0 {
		return 0, false
	}
	return *r.Used / *r.Window * 100, true
}
func validCount(n *float64) *float64 {
	if n == nil || *n < 0 || math.IsNaN(*n) || math.IsInf(*n, 0) {
		return nil
	}
	return n
}

var contextCount = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)([kKmM]?)$`)
var contextTotals = regexp.MustCompile(`(?m)^\*\*Tokens:\*\*\s+([0-9.,]+[kKmM]?)\s*/\s*([0-9.,]+[kKmM]?)(?:\s+\([0-9.]+%\))?\s*$`)
var contextModel = regexp.MustCompile(`(?m)^\*\*Model:\*\*\s+(.+?)\s*$`)

func parseContextCount(s string) *float64 {
	s = strings.TrimSpace(strings.Trim(s, "*`"))
	s = strings.TrimSpace(strings.TrimSuffix(s, " tokens"))
	m := contextCount.FindStringSubmatch(strings.ReplaceAll(s, ",", ""))
	if m == nil {
		return nil
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	switch strings.ToLower(m[2]) {
	case "k":
		n *= 1000
	case "m":
		n *= 1000000
	}
	return validCount(&n)
}

// ParseClaudeContext parses only a correlated native /context response. Unknown
// formats remain available in Raw; arbitrary assistant prose is not interpreted.
func ParseClaudeContext(text string) ContextReport {
	r := ContextReport{Provider: "claude", Basis: "Native /context snapshot"}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	model, totals := contextModel.FindStringSubmatch(text), contextTotals.FindStringSubmatch(text)
	if !strings.Contains(text, "## Context Usage") || model == nil || totals == nil {
		r.Note = "Context response format was not recognized. Open Raw to inspect the provider response."
		return r
	}
	r.Model = model[1]
	r.Used, r.Window = parseContextCount(totals[1]), parseContextCount(totals[2])
	if r.Window != nil && *r.Window == 0 {
		r.Window = nil
	}
	// Read only the category summary table. Per-tool and per-file tables are not
	// context partitions and must never be added to these values.
	inTable := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			if inTable {
				break
			}
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) < 2 {
			continue
		}
		name := strings.TrimSpace(strings.Trim(strings.TrimSpace(cells[0]), "*`"))
		if strings.EqualFold(name, "Category") && strings.EqualFold(strings.TrimSpace(cells[1]), "Tokens") {
			inTable = true
			continue
		}
		if !inTable {
			continue
		}
		n := parseContextCount(strings.TrimSpace(cells[1]))
		if n == nil || name == "" || strings.EqualFold(name, "Total") {
			continue
		}
		r.Categories = append(r.Categories, ContextMetric{Name: name, Tokens: *n})
	}
	r.Note = "Provider snapshot; not a live count. Category values may be rounded."
	return r
}

func ParseCodexContext(raw []byte) ContextReport {
	r := ContextReport{Provider: "codex", Basis: "Last reported turn"}
	var v struct {
		TokenUsage *struct {
			ModelContextWindow *float64
			Last               *struct{ TotalTokens, InputTokens, OutputTokens, CachedInputTokens, ReasoningOutputTokens *float64 }
		}
	}
	if json.Unmarshal(raw, &v) != nil || v.TokenUsage == nil {
		r.Note = "Context response format was not recognized. Open Raw to inspect the provider response."
		return r
	}
	u := v.TokenUsage
	r.Window = validCount(u.ModelContextWindow)
	if r.Window != nil && *r.Window == 0 {
		r.Window = nil
	}
	if l := u.Last; l != nil {
		r.Used = validCount(l.TotalTokens)
		for _, m := range []struct {
			name   string
			value  *float64
			subset bool
		}{
			{"Input", l.InputTokens, false}, {"Output", l.OutputTokens, false},
			{"Cached input (within input)", l.CachedInputTokens, true},
			{"Reasoning (within output)", l.ReasoningOutputTokens, true},
		} {
			if n := validCount(m.value); n != nil {
				r.Metrics = append(r.Metrics, ContextMetric{Name: m.name, Tokens: *n, Subset: m.subset})
			}
		}
	}
	r.Note = "Last reported turn, not a live token count. System/tool category breakdown is not reported."
	return r
}
