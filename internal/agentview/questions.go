package agentview

import (
	"encoding/json"
	"fmt"
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

// EncodeQuestionAnswers preserves the existing reply API; only Claude supports
// multiple selections here, represented by its comma-separated answer string.
func EncodeQuestionAnswers(qs []Question, selections [][]bool, other []string) (string, error) {
	if len(qs) != len(selections) || len(qs) != len(other) {
		return "", fmt.Errorf("incomplete answers")
	}
	values := map[string]string{}
	for i, q := range qs {
		var selected []string
		for j, o := range q.Options {
			if j < len(selections[i]) && selections[i][j] {
				selected = append(selected, o.Label)
			}
		}
		if q.Other && strings.TrimSpace(other[i]) != "" {
			selected = append(selected, other[i])
		}
		if len(selected) == 0 || (!q.Multi && len(selected) != 1) {
			return "", fmt.Errorf("answer question %d before submitting", i+1)
		}
		values[q.Key] = strings.Join(selected, ", ")
	}
	b, err := json.Marshal(values)
	return string(b), err
}
