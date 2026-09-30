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
	model, _ := m.modelStatus()
	return model
}

// modelStatus reads the newest catalog this run confirmed, so one walk answers
// both what the quota belongs to and what the composer shows. Without a
// catalog, the model the session was created with is all there is to say, and
// the reasoning level is unknown rather than assumed.
func (m *model) modelStatus() (model, effort string) {
	s := m.current()
	if s == nil {
		return "", ""
	}
	model = s.Model
	for i := len(m.events[s.Id]) - 1; i >= 0; i-- {
		e := m.events[s.Id][i]
		if e.Kind != "models" || e.RunId != "" && e.RunId != s.RunId {
			continue
		}
		var v modelCatalog
		if json.Unmarshal(e.Payload, &v) != nil {
			continue
		}
		effort = v.currentEffort()
		model = v.Model
		if model == "" || model == "default" {
			for _, o := range v.Models {
				if o.Default {
					return o.ID, effort
				}
			}
		}
		return model, effort
	}
	return model, ""
}

// The composer's left slot says what is about to run: the model, and the
// reasoning level when the provider reports one. Permission mode used to sit
// here, but it is a policy saved once and read back from the server -- not
// something that changes under the reader while they type.
func (m *model) agentStatus() string {
	model, effort := m.modelStatus()
	if model == "" {
		return ""
	}
	out := " " + accent.Render(pickerLabel(model))
	if effort != "" && effort != "default" {
		out += muted.Render(" · " + pickerLabel(effort))
	}
	return out
}
