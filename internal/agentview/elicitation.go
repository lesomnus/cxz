package agentview

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/lesomnus/cxz/internal/core"
)

const CodexElicitation = core.CodexElicitation
const elicitationActionKey = "cxz:action"

type elicitation struct {
	ServerName string          `json:"serverName"`
	Mode       string          `json:"mode"`
	Message    string          `json:"message"`
	URL        string          `json:"url"`
	Schema     json.RawMessage `json:"requestedSchema"`
}

func readElicitation(raw []byte) (elicitation, error) {
	var r struct {
		Params elicitation `json:"params"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return r.Params, err
	}
	switch r.Params.Mode {
	case "form", "url":
	default:
		return r.Params, fmt.Errorf("unsupported MCP elicitation mode %q", r.Params.Mode)
	}
	return r.Params, nil
}

func ElicitationQuestions(raw []byte) ([]Question, error) {
	p, err := readElicitation(raw)
	if err != nil {
		return nil, err
	}
	text := p.Message
	if p.Mode == "url" {
		text += "\n" + p.URL + "\nComplete the URL flow yourself, then accept. cxz does not open it automatically."
	}
	out := []Question{{Key: elicitationActionKey, Header: "MCP · " + p.ServerName, Text: text, ElicitationAction: true, Options: []QuestionOption{{Label: "Accept"}, {Label: "Decline"}, {Label: "Cancel"}}}}
	if p.Mode == "url" {
		return out, nil
	}
	var schema struct {
		Type       string
		Properties map[string]json.RawMessage
		Required   []string
	}
	if err := json.Unmarshal(p.Schema, &schema); err != nil || schema.Type != "object" {
		return nil, fmt.Errorf("MCP form requires an object schema")
	}
	required := map[string]bool{}
	for _, key := range schema.Required {
		required[key] = true
	}
	keys := []string{}
	for key := range schema.Properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		f := object(schema.Properties[key])
		typ := f.text("type")
		title := f.text("title")
		if title == "" {
			title = key
		}
		q := Question{Key: "field:" + key, Header: p.ServerName, Text: title, Other: true, Optional: !required[key]}
		if desc := f.text("description"); desc != "" {
			q.Text += "\n" + desc
		}
		if q.Optional {
			q.Text += "\nOptional: leave blank to omit."
		}
		var values []string
		if typ == "boolean" {
			values = []string{"true", "false"}
		} else {
			_ = json.Unmarshal(f["enum"], &values)
		}
		var choices []struct {
			Const string
			Title string
		}
		_ = json.Unmarshal(f["oneOf"], &choices)
		var enumNames []string
		_ = json.Unmarshal(f["enumNames"], &enumNames)
		for i, v := range values {
			description := ""
			if i < len(enumNames) {
				description = enumNames[i]
			}
			q.Options = append(q.Options, QuestionOption{Label: v, Description: description})
		}
		for _, v := range choices {
			q.Options = append(q.Options, QuestionOption{Label: v.Const, Description: v.Title})
		}
		if len(q.Options) > 0 {
			q.Other = false
		}
		if typ != "string" && typ != "boolean" {
			q.Text += "\nEnter a JSON " + typ + " value."
		}
		out = append(out, q)
	}
	return out, nil
}

// Decline/cancel do not require (or send) partially completed form fields.
func ElicitationDismissed(qs []Question, values map[string]core.AnswerSelection) bool {
	if len(qs) == 0 || !qs[0].ElicitationAction {
		return false
	}
	a := values[qs[0].Key]
	return a.Other == "" && len(a.Selected) == 1 && (a.Selected[0] == "Decline" || a.Selected[0] == "Cancel")
}

func ElicitationResponse(raw []byte, allow bool, values map[string]core.AnswerSelection, answers map[string]string) (map[string]any, error) {
	result := map[string]any{"action": "decline", "content": nil}
	if !allow {
		return result, nil
	}
	qs, err := ElicitationQuestions(raw)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		values = map[string]core.AnswerSelection{}
		for _, q := range qs {
			a := answers[q.Key]
			if q.Other {
				values[q.Key] = core.AnswerSelection{Other: a}
			} else if a != "" {
				values[q.Key] = core.AnswerSelection{Selected: []string{a}}
			} else {
				values[q.Key] = core.AnswerSelection{}
			}
		}
	}
	values, err = NormalizeAnswers(qs, values)
	if err != nil {
		return nil, err
	}
	action := strings.ToLower(values[elicitationActionKey].Selected[0])
	result["action"] = action
	if action != "accept" {
		return result, nil
	}
	p, _ := readElicitation(raw)
	if p.Mode == "url" {
		return result, nil
	}
	props := object(p.Schema).child("properties")
	content := map[string]any{}
	for _, q := range qs[1:] {
		a := values[q.Key]
		value := a.Other
		if len(a.Selected) > 0 {
			value = a.Selected[0]
		}
		if value == "" && q.Optional {
			continue
		}
		key := strings.TrimPrefix(q.Key, "field:")
		if props.child(key).text("type") == "string" {
			content[key] = value
		} else {
			var v any
			if err := json.Unmarshal([]byte(value), &v); err != nil {
				return nil, fmt.Errorf("invalid value for %s: %w", key, err)
			}
			content[key] = v
		}
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(p.Schema, &schema); err != nil {
		return nil, err
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil, err
	}
	if err := resolved.Validate(content); err != nil {
		return nil, fmt.Errorf("MCP form: %w", err)
	}
	result["content"] = content
	return result, nil
}

// ElicitationButtons identifies decisions that need no text/form fields. URL
// confirmations also use buttons, but cannot be completed by the FULL policy.
func ElicitationButtons(raw []byte) bool {
	p, err := readElicitation(raw)
	if err != nil {
		return false
	}
	if p.Mode == "url" {
		return true
	}
	qs, err := ElicitationQuestions(raw)
	if err != nil || len(qs) != 1 {
		return false
	}
	_, err = ElicitationResponse(raw, true, ElicitationSelection("Accept"), nil)
	return err == nil
}
func ElicitationSelection(action string) map[string]core.AnswerSelection {
	return map[string]core.AnswerSelection{elicitationActionKey: {Selected: []string{action}}}
}
func QuestionRequest(name string, raw []byte) bool {
	return core.Question(name) && !(name == CodexElicitation && ElicitationButtons(raw))
}
func AutomaticApproval(name string, raw []byte) bool {
	if name == CodexElicitation {
		p, err := readElicitation(raw)
		return err == nil && p.Mode == "form" && ElicitationButtons(raw)
	}
	return core.AutomaticApproval(name)
}
