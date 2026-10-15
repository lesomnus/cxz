// Deterministic stream-json fixture. Not linked into cxz.
package main

import (
	"bufio"
	"crypto/sha256"
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
	resume, fork := "", false
	for i, arg := range os.Args {
		if arg == "--resume" && i+1 < len(os.Args) {
			resume = os.Args[i+1]
		}
		if arg == "--fork-session" {
			fork = true
		}
	}
	if resume != "" {
		vendor = resume
	}
	native := func(id string) string {
		return filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "projects", "fixture", id+".jsonl")
	}
	if fork {
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(os.Getenv("CLAUDE_CONFIG_DIR"))))
		vendor = hash[:8] + "-" + hash[8:12] + "-" + hash[12:16] + "-" + hash[16:20] + "-" + hash[20:32]
		previous, err := os.ReadFile(native(resume))
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(native(vendor), previous, 0600); err != nil {
			panic(err)
		}
	}

	var nativeMu sync.Mutex
	emit := func(v any) { mu.Lock(); defer mu.Unlock(); b, _ := json.Marshal(v); fmt.Println(string(b)) }
	finish := func(text string) {
		nativeMu.Lock()
		os.MkdirAll(filepath.Dir(native(vendor)), 0700)
		if file, err := os.OpenFile(native(vendor), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); err == nil {
			json.NewEncoder(file).Encode(map[string]string{"text": text})
			file.Close()
		}
		nativeMu.Unlock()
		emit(map[string]any{"type": "assistant", "session_id": vendor, "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}})
		emit(map[string]any{"type": "result", "subtype": "success", "session_id": vendor, "result": text})
	}
	pending := ""
	requests := 0
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
			response := map[string]any{"subtype": "success", "request_id": v.RequestID}
			if v.Request.Subtype == "get_usage" {
				response["response"] = map[string]any{"rate_limits": map[string]any{"five_hour": map[string]any{"utilization": 60, "resets_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}, "seven_day": map[string]any{"utilization": 54, "resets_at": time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)}}}
			}
			emit(map[string]any{"type": "control_response", "response": response})
			if v.Request.Subtype == "interrupt" {
				finish("interrupted")
			}
		case "user":
			emit(map[string]any{"type": "system", "subtype": "init", "session_id": vendor})
			switch {
			case v.Message.Content == "/context":
				finish("Context fixture · 2k / 10k tokens")
			case v.Message.Content == "/compact":
				emit(map[string]any{"type": "system", "subtype": "compact_boundary", "compact_metadata": map[string]any{"trigger": "manual", "pre_tokens": 2000}})
				finish("Compacted fixture context")
			case v.Message.Content == "account-context":
				finish("profile=" + filepath.Base(filepath.Dir(os.Getenv("CLAUDE_CONFIG_DIR"))) + " home=" + filepath.Base(filepath.Dir(os.Getenv("HOME"))) + " inherited-key=" + fmt.Sprint(os.Getenv("OPENAI_API_KEY") != "" || os.Getenv("ANTHROPIC_API_KEY") != ""))
			case v.Message.Content == "fork-context":
				history, _ := os.ReadFile(native(vendor))
				finish("inherited hello=" + fmt.Sprint(strings.Contains(string(history), "echo: hello")))
			case strings.HasPrefix(v.Message.Content, "approval"), v.Message.Content == "question":
				pending = v.Message.Content
				tool := "Bash"
				input := map[string]any{"command": "printf test"}
				if pending == "question" {
					tool = "AskUserQuestion"
					input = map[string]any{"questions": []any{map[string]any{"question": "Choose a color", "options": []any{map[string]any{"label": "Blue"}, map[string]any{"label": "Green"}}}}}
				}
				if pending == "approval delayed" {
					time.Sleep(700 * time.Millisecond)
				}
				requests++
				emit(map[string]any{"type": "control_request", "request_id": fmt.Sprintf("request-%d", requests), "request": map[string]any{"subtype": "can_use_tool", "tool_name": tool, "input": input}})
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
