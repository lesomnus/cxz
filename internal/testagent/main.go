// Deterministic stream-json fixture. Not linked into cxz.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

func main() {
	var mu sync.Mutex
	vendor := "00000000-0000-4000-8000-000000000001"
	emit := func(v any) { mu.Lock(); defer mu.Unlock(); b, _ := json.Marshal(v); fmt.Println(string(b)) }
	finish := func(text string) {
		emit(map[string]any{"type": "assistant", "session_id": vendor, "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}})
		emit(map[string]any{"type": "result", "subtype": "success", "session_id": vendor, "result": text})
	}
	pending := ""
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var v struct {
			Type      string `json:"type"`
			RequestID string `json:"request_id"`
			Request   struct {
				Subtype string `json:"subtype"`
			} `json:"request"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Response struct {
				Response struct {
					Behavior string         `json:"behavior"`
					Input    map[string]any `json:"updatedInput"`
				} `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal(sc.Bytes(), &v) != nil {
			continue
		}
		switch v.Type {
		case "control_request":
			emit(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": v.RequestID}})
			if v.Request.Subtype == "interrupt" {
				finish("interrupted")
			}
		case "user":
			emit(map[string]any{"type": "system", "subtype": "init", "session_id": vendor})
			switch {
			case strings.HasPrefix(v.Message.Content, "approval"), v.Message.Content == "question":
				pending = v.Message.Content
				tool := "Bash"
				input := map[string]any{"command": "printf test"}
				if pending == "question" {
					tool = "AskUserQuestion"
					input = map[string]any{"questions": []any{map[string]any{"question": "Choose a color", "options": []any{map[string]any{"label": "Blue"}, map[string]any{"label": "Green"}}}}}
				}
				emit(map[string]any{"type": "control_request", "request_id": "request-1", "request": map[string]any{"subtype": "can_use_tool", "tool_name": tool, "input": input}})
			case v.Message.Content == "slow":
				go func() { time.Sleep(2 * time.Second); finish("slow complete") }()
			case v.Message.Content == "wait":
			default:
				finish("echo: " + v.Message.Content)
			}
		case "control_response":
			finish(pending + ": " + v.Response.Response.Behavior)
		}
	}
}
