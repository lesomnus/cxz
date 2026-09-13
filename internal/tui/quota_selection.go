package tui

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode"

	"github.com/lesomnus/cxz/internal/agentview"
)

func quotaModelKey(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

// Match complete provider bucket IDs/names, never a guessed model substring.
// All nonselected buckets remain counted and accessible in /usage.
func statusQuotaWindows(provider, model string, all []agentview.Window) ([]agentview.Window, int) {
	var selected []agentview.Window
	matching := false
	key := quotaModelKey(model)
	if provider == "codex" && key != "" && key != "default" {
		for _, w := range all {
			if w.Bucket != "" && w.Bucket != "codex" && (quotaModelKey(w.Bucket) == key || quotaModelKey(w.BucketName) == key) {
				matching = true
			}
		}
	}
	for _, w := range all {
		include := true
		switch provider {
		case "codex":
			if matching {
				include = w.Bucket != "codex" && (quotaModelKey(w.Bucket) == key || quotaModelKey(w.BucketName) == key)
			} else {
				include = w.Bucket == "codex" || strings.HasPrefix(w.Key, "codex/")
			}
			if include && w.Period != "" {
				w.Label = w.Period
			}
		case "claude":
			include = w.Key == "five_hour" || w.Key == "seven_day"
		}
		if include {
			selected = append(selected, w)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		// Prefer shorter primary windows without reordering by changing usage.
		rank := func(w agentview.Window) int {
			if w.Label == "5h" {
				return 0
			}
			if w.Label == "wk" {
				return 1
			}
			return 2
		}
		return rank(selected[i]) < rank(selected[j])
	})
	return selected, len(all) - len(selected)
}

// Resolve provider default through the current run's catalog, not its display
// label (which may also include effort). The latest confirmed catalog wins.
func (m *model) quotaModel() string {
	s := m.current()
	if s == nil {
		return ""
	}
	model := s.Model
	for i := len(m.events[s.Id]) - 1; i >= 0; i-- {
		e := m.events[s.Id][i]
		if e.Kind != "models" || e.RunId != "" && e.RunId != s.RunId {
			continue
		}
		var v struct {
			Model  string
			Models []agentview.ModelOption
		}
		if json.Unmarshal(e.Payload, &v) != nil {
			continue
		}
		model = v.Model
		if model == "" || model == "default" {
			for _, o := range v.Models {
				if o.Default {
					return o.ID
				}
			}
		}
		return model
	}
	return model
}
