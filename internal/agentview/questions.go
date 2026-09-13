package agentview

import (
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"strings"
)

// Question is a provider-neutral form. Key remains the native reply key:
// Claude uses the question text; Codex uses the stable question ID.
type Question struct {
	Key, Header, Text    string
	Multi, Other, Secret bool
	Options              []QuestionOption
}
type QuestionOption struct{ Label, Description, Preview string }

func Questions(provider, method string, raw []byte) ([]Question, error) {
	root := object(raw)
	var body fields
	switch {
	case provider == "claude" && method == "AskUserQuestion":
		body = root.child("input")
	case provider == "codex" && method == "item/tool/requestUserInput":
		body = root.child("params")
	default:
		return nil, fmt.Errorf("unsupported question provider/method")
	}
	var wire []struct {
		ID, Header, Question string
		MultiSelect          bool `json:"multiSelect"`
		IsOther              bool `json:"isOther"`
		IsSecret             bool `json:"isSecret"`
		Options              []QuestionOption
	}
	if err := json.Unmarshal(body["questions"], &wire); err != nil {
		return nil, fmt.Errorf("invalid questions: %w", err)
	}
	if len(wire) == 0 {
		return nil, fmt.Errorf("empty questions")
	}
	seen := map[string]bool{}
	var out []Question
	for _, w := range wire {
		key := w.ID
		if provider == "claude" {
			key = w.Question
		}
		if key == "" || w.Question == "" || seen[key] {
			return nil, fmt.Errorf("missing or duplicate question key")
		}
		seen[key] = true
		labels := map[string]bool{}
		for _, o := range w.Options {
			if strings.TrimSpace(o.Label) == "" || labels[o.Label] {
				return nil, fmt.Errorf("missing or duplicate option label")
			}
			labels[o.Label] = true
		}
		out = append(out, Question{Key: key, Header: w.Header, Text: w.Question, Multi: provider == "claude" && w.MultiSelect, Other: provider == "claude" || w.IsOther || len(w.Options) == 0, Secret: w.IsSecret, Options: w.Options})
	}
	return out, nil
}

// Keep choices distinct until the provider adapter creates its native reply.
func EncodeQuestionAnswers(qs []Question, selections [][]bool, other []string) (string, error) {
	if len(qs) != len(selections) || len(qs) != len(other) {
		return "", fmt.Errorf("incomplete answers")
	}
	values := map[string]core.AnswerSelection{}
	for i, q := range qs {
		var selected []string
		for j, o := range q.Options {
			if j < len(selections[i]) && selections[i][j] {
				selected = append(selected, o.Label)
			}
		}
		values[q.Key] = core.AnswerSelection{Selected: selected, Other: other[i]}
	}
	values, err := NormalizeAnswers(qs, values)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(values)
	return string(b), err
}

func NormalizeAnswers(qs []Question, values map[string]core.AnswerSelection) (map[string]core.AnswerSelection, error) {
	if len(values) != len(qs) {
		return nil, fmt.Errorf("answer every question")
	}
	out := map[string]core.AnswerSelection{}
	for i, q := range qs {
		a, ok := values[q.Key]
		if !ok {
			return nil, fmt.Errorf("missing answer %d", i+1)
		}
		labels := map[string]bool{}
		for _, o := range q.Options {
			labels[o.Label] = true
		}
		seen := map[string]bool{}
		clean := core.AnswerSelection{Selected: []string{}, Other: a.Other}
		if !q.Secret {
			clean.Other = strings.TrimSpace(clean.Other)
		}
		for _, label := range a.Selected {
			if !labels[label] {
				return nil, fmt.Errorf("unknown option for question %d", i+1)
			}
			if !seen[label] {
				clean.Selected = append(clean.Selected, label)
				seen[label] = true
			}
		}
		if clean.Other != "" {
			if !q.Other {
				return nil, fmt.Errorf("Other is not supported for question %d", i+1)
			}
			if labels[clean.Other] {
				if !seen[clean.Other] {
					clean.Selected = append(clean.Selected, clean.Other)
				}
				clean.Other = ""
			}
		}
		count := len(clean.Selected)
		if clean.Other != "" {
			count++
		}
		if count == 0 || (!q.Multi && count != 1) {
			return nil, fmt.Errorf("answer question %d before submitting", i+1)
		}
		out[q.Key] = clean
	}
	return out, nil
}

// Claude requires string answers. Notes preserve the exact selection/Other
// boundary (including commas); preview comes only from the pending option.
func ClaudeAnswerAnnotations(qs []Question, values map[string]core.AnswerSelection) (map[string]string, map[string]map[string]string) {
	answers := map[string]string{}
	annotations := map[string]map[string]string{}
	for _, q := range qs {
		a := values[q.Key]
		parts := append([]string{}, a.Selected...)
		if a.Other != "" {
			parts = append(parts, a.Other)
		}
		answers[q.Key] = strings.Join(parts, ", ")
		note := map[string]string{}
		if a.Other != "" || q.Multi {
			b, _ := json.Marshal(a)
			note["notes"] = "cxz structured selection: " + string(b)
		}
		if !q.Multi && len(a.Selected) == 1 {
			for _, o := range q.Options {
				if o.Label == a.Selected[0] && o.Preview != "" {
					note["preview"] = o.Preview
				}
			}
		}
		if len(note) > 0 {
			annotations[q.Key] = note
		}
	}
	return answers, annotations
}
