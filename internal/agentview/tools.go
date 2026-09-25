package agentview

import (
	"encoding/json"
	"strings"
)

// ToolActivity is display-only: native names/payloads still drive execution.
// Counts describe supplied content/replacement spans unless a provider diff
// supplies actual hunks. Never read the current file to guess an earlier diff.
type ToolActivity struct {
	Kind, Command, Description string
	Files                      []FileActivity
}
type FileActivity struct {
	Path, MovePath, Action string
	Added, Removed, Lines  int
	Measure                string // content, replacement, diff, or unknown
	PerMatch               bool
}

func ToolView(provider, name string, raw []byte) (ToolActivity, bool) {
	p := object(raw)
	if provider == "claude" {
		switch name {
		case "Write":
			var content string
			known := json.Unmarshal(p["content"], &content) == nil
			measure := "unknown"
			if known {
				measure = "content"
			}
			return ToolActivity{Kind: "files", Files: []FileActivity{{Path: p.text("file_path"), Action: "write", Lines: textLines(content), Measure: measure}}}, true
		case "Edit":
			var old, next string
			known := json.Unmarshal(p["old_string"], &old) == nil && json.Unmarshal(p["new_string"], &next) == nil
			measure := "unknown"
			if known {
				measure = "replacement"
			}
			return ToolActivity{Kind: "files", Files: []FileActivity{{Path: p.text("file_path"), Action: "edit", Removed: textLines(old), Added: textLines(next), Measure: measure, PerMatch: string(p["replace_all"]) == "true"}}}, true
		case "Bash":
			return ToolActivity{Kind: "command", Command: p.text("command"), Description: p.text("description")}, true
		case "Read":
			return ToolActivity{Kind: "read", Files: []FileActivity{{Path: p.text("file_path"), Action: "read"}}}, true
		case "Skill":
			description := "Skill"
			for _, key := range []string{"skill", "args"} {
				if value := strings.TrimSpace(p.text(key)); value != "" {
					description += " " + value
				}
			}
			return ToolActivity{Kind: "tool", Description: description}, true
		}
	}
	if provider == "codex" {
		item := p.child("item")
		switch item.text("type") {
		case "commandExecution":
			return ToolActivity{Kind: "command", Command: item.text("command")}, true
		case "fileChange":
			var changes []struct {
				Path, Diff string
				Kind       json.RawMessage
			}
			if json.Unmarshal(item["changes"], &changes) != nil {
				return ToolActivity{}, false
			}
			activity := ToolActivity{Kind: "files"}
			for _, change := range changes {
				kind := object(change.Kind)
				action := kind.text("type")
				if action == "" {
					_ = json.Unmarshal(change.Kind, &action)
				}
				if action == "" {
					action = "edit"
				}
				added, removed, known := diffCounts(change.Diff)
				measure := "unknown"
				if known {
					measure = "diff"
				}
				activity.Files = append(activity.Files, FileActivity{Path: change.Path, MovePath: kind.text("move_path"), Action: action, Added: added, Removed: removed, Measure: measure})
			}
			return activity, true
		}
	}
	return ToolActivity{}, false
}

func textLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(strings.TrimSuffix(text, "\n"), "\n") + 1
}

func diffCounts(diff string) (added, removed int, known bool) {
	inHunk := false
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "@@ ") {
			known = true
			inHunk = true
			continue
		}
		if strings.HasPrefix(line, "diff --git ") {
			inHunk = false
			continue
		}
		if !inHunk || line == "" {
			continue
		}
		switch line[0] {
		case '+':
			added++
		case '-':
			removed++
		}
	}
	return
}
