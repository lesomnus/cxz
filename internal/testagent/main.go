// Deterministic stream-json fixture. Not linked into cxz.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func main() {
	if len(os.Args) > 2 && os.Args[1] == "auth" && os.Args[2] == "login" {
		config := os.Getenv("CLAUDE_CONFIG_DIR")
		b, _ := json.Marshal(map[string]any{"claudeAiOauth": map[string]string{"accessToken": "synthetic-" + filepath.Base(filepath.Dir(config))}})
		if err := os.WriteFile(filepath.Join(config, ".credentials.json"), b, 0600); err != nil {
			panic(err)
		}
		fmt.Println("Synthetic fixture login complete (not a real vendor login)")
		return
	}
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
			case v.Message.Content == "account-context":
				finish("profile=" + filepath.Base(filepath.Dir(os.Getenv("CLAUDE_CONFIG_DIR"))) + " home=" + filepath.Base(filepath.Dir(os.Getenv("HOME"))) + " inherited-key=" + fmt.Sprint(os.Getenv("OPENAI_API_KEY") != "" || os.Getenv("ANTHROPIC_API_KEY") != ""))
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
			case v.Message.Content == "spawn-child":
				child := exec.Command("sh", "-c", "sleep 2; printf orphan > orphan.txt")
				if child.Start() == nil {
					go child.Wait()
				}
				emit(map[string]any{"type": "assistant", "session_id": vendor, "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "child started"}}}})
			default:
				finish("echo: " + v.Message.Content)
			}
		case "control_response":
			finish(pending + ": " + v.Response.Response.Behavior)
		}
	}
}
