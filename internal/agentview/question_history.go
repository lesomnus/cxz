package agentview

import (
	"encoding/json"

	"github.com/lesomnus/cxz/internal/core"
)

// QuestionResolution records the user's answer independently of provider wire
// formats. Secret fields are redacted in the reading history.
type QuestionResolution struct {
	State   string                          `json:"state"`
	Answers map[string]core.AnswerSelection `json:"answers,omitempty"`
}

func QuestionReply(provider, method string, raw []byte, state string, answers map[string]string, selections map[string]core.AnswerSelection) *QuestionResolution {
	if method == "" {
		var wire struct {
			Method string `json:"method"`
			Tool   string `json:"tool_name"`
		}
		_ = json.Unmarshal(raw, &wire)
		method = wire.Method
		if method == "" {
			method = wire.Tool
		}
	}
	qs, err := Questions(provider, method, raw)
	if err != nil {
		return nil
	}
	out := &QuestionResolution{State: state}
	if state != "allowed" {
		return out
	}
	out.Answers = map[string]core.AnswerSelection{}
	if normalized, err := NormalizeAnswers(qs, selections); err == nil {
		selections = normalized
	}
	for _, q := range qs {
		value, ok := selections[q.Key]
		if !ok {
			value.Other = answers[q.Key]
			if value.Other == "" {
				value.Other = answers[q.Text]
			}
		}
		if q.Secret {
			value = core.AnswerSelection{Other: "[redacted]"}
		}
		out.Answers[q.Key] = value
	}
	return out
}
