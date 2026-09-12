// Package agentview projects provider payloads into read-only display models.
// Original events and approval decisions always retain their native payloads.
package agentview

import (
	"encoding/json"
	"strings"
)

type Approval struct{ Title, Detail string }

func ApprovalView(provider, name string, raw []byte) Approval {
	root := object(raw)
	p := root
	title := name
	if provider == "codex" {
		p = root.child("params")
		switch name {
		case "item/commandExecution/requestApproval":
			title = "Command"
		case "item/fileChange/requestApproval":
			title = "Files"
		case "item/permissions/requestApproval":
			title = "Permissions"
		case "item/tool/requestUserInput":
			title = "Question"
		}
	}
	var lines []string
	for _, key := range []string{"description", "reason", "command", "cwd"} {
		if text := p.text(key); text != "" {
			lines = append(lines, key+": "+text)
		}
	}
	if command := p.child("input").text("command"); command != "" {
		lines = append(lines, "$ "+command)
	}
	// Keep every provider field accessible, including unsupported request types.
	var value any
	detail := string(raw)
	if json.Unmarshal(raw, &value) == nil {
		b, _ := json.MarshalIndent(value, "", "  ")
		detail = string(b)
	}
	if len(lines) > 0 {
		detail = strings.Join(lines, "\n") + "\n\n" + detail
	}
	return Approval{Title: title, Detail: detail}
}

type fields map[string]json.RawMessage

func object(raw []byte) fields           { var f fields; _ = json.Unmarshal(raw, &f); return f }
func (f fields) child(key string) fields { return object(f[key]) }
func (f fields) text(key string) string  { var s string; _ = json.Unmarshal(f[key], &s); return s }
func (f fields) number(key string) (float64, bool) {
	var n float64
	raw, ok := f[key]
	valid := ok && string(raw) != "null" && json.Unmarshal(raw, &n) == nil
	return n, valid
}
