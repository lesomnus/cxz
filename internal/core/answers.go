package core

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type AnswerSelection struct {
	Selected []string `json:"selected"`
	Other    string   `json:"other,omitempty"`
}

// Preserve structured selections through the manager/runtime command boundary.
// String maps remain accepted for the explicit, advanced CLI reply command.
func DecodeAnswers(raw string) (map[string]string, map[string]AnswerSelection, error) {
	legacy := map[string]string{}
	choices := map[string]AnswerSelection{}
	if raw == "" {
		return legacy, choices, nil
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &entries); err != nil || entries == nil {
		return nil, nil, fmt.Errorf("answers_json must be an object")
	}
	for key, value := range entries {
		if key == "" {
			return nil, nil, fmt.Errorf("empty answer key")
		}
		var text string
		if len(value) > 0 && value[0] == '"' && json.Unmarshal(value, &text) == nil {
			legacy[key] = text
			continue
		}
		var answer AnswerSelection
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.DisallowUnknownFields()
		if len(value) == 0 || value[0] != '{' || decoder.Decode(&answer) != nil {
			return nil, nil, fmt.Errorf("answer must be a string or {selected: [...], other: string}")
		}
		choices[key] = answer
	}
	if len(legacy) > 0 && len(choices) > 0 {
		return nil, nil, fmt.Errorf("do not mix string and structured answers")
	}
	return legacy, choices, nil
}
