package supervisor

import (
	"encoding/json"
	"fmt"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
)

func (s *Supervisor) hasBlockingPending() bool {
	for _, p := range s.pending {
		if p.Text != agentview.CodexAsyncQuestion {
			return true
		}
	}
	return false
}

// An async agentMessage is a notification, not a server request. Its answer is
// standalone tool output: app-server queues it if the original turn is active.
func (c *codexProtocol) answerAsync(p core.Event, command core.Command) (any, error) {
	if !command.Allow {
		return nil, nil
	}
	if c.s.snap.State != "idle" && c.s.snap.State != "working" {
		return nil, fmt.Errorf("answer async question after the blocking request is resolved")
	}
	qs, err := agentview.CodexAsyncQuestions(p.Payload)
	if err != nil {
		return nil, err
	}
	values := command.Selections
	if len(values) == 0 {
		values = map[string]core.AnswerSelection{}
		for _, q := range qs {
			values[q.Key] = core.AnswerSelection{Other: command.Answers[q.Key]}
		}
	}
	values, err = agentview.NormalizeAnswers(qs, values)
	if err != nil {
		return nil, err
	}
	type answer struct {
		Question string   `json:"question"`
		Selected []string `json:"selected"`
		Other    string   `json:"other,omitempty"`
	}
	answers := make([]answer, 0, len(qs))
	for _, q := range qs {
		a := values[q.Key]
		answers = append(answers, answer{q.Text, a.Selected, a.Other})
	}
	var native struct {
		Item struct {
			ID string `json:"id"`
		} `json:"item"`
	}
	_ = json.Unmarshal(p.Payload, &native)
	body, _ := json.Marshal(map[string]any{"question_message_id": native.Item.ID, "answers": answers})
	if c.asyncReplies == nil {
		c.asyncReplies = map[string]core.Event{}
	}
	responseID, _ := json.Marshal(command.ClientID)
	c.asyncReplies[string(responseID)] = p
	return rpc(command.ClientID, "turn/start", map[string]any{
		"threadId": c.s.snap.VendorID, "input": []any{},
		"toolOutput": map[string]any{"name": "request_user_input_async", "output": string(body)},
	}), nil
}
