package auxiliary

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/agentview"
	"io"
	"os/exec"
	"strings"
	"time"
)

// Provider starts only an isolated helper's provider executable. It never resumes threads.
func Provider(ctx context.Context, bin, dir string, auth accounts.LaunchAuth, in Input, token func(string, bool) (accounts.Token, error)) (out Output, err error) {
	args := append([]string{}, auth.Args...)
	if in.Profile.Agent == "codex" {
		args = append(args, "app-server", "--listen", "stdio://")
	} else {
		args = append(args, "--print", "--verbose", "--input-format", "stream-json", "--output-format", "stream-json", "--tools", "", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--setting-sources", "", "--no-session-persistence", "--system-prompt", Instructions, "--settings", `{"disableAllHooks":true}`)
		if in.Profile.Model != "" {
			args = append(args, "--model", in.Profile.Model)
		}
		if in.Profile.Effort != "" {
			args = append(args, "--effort", in.Profile.Effort)
		}
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(child, bin, args...)
	cmd.Env = auth.Env
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	stdin, e := cmd.StdinPipe()
	if e != nil {
		return out, e
	}
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		return out, e
	}
	// Never echo raw provider stderr: it may include credential material.
	cmd.Stderr = io.Discard
	if e = cmd.Start(); e != nil {
		return out, e
	}
	defer func() { cancel(); _ = stdin.Close(); _ = cmd.Wait() }()
	write := func(v any) error { return json.NewEncoder(stdin).Encode(v) }
	rpc := func(id, method string, p any) error {
		return write(map[string]any{"id": id, "method": method, "params": p})
	}
	if in.Profile.Agent == "codex" {
		err = rpc("init", "initialize", map[string]any{"clientInfo": map[string]any{"name": "cxz-auxiliary", "version": "1"}, "capabilities": map[string]any{"experimentalApi": true}})
	} else {
		err = write(map[string]any{"type": "control_request", "request_id": "init", "request": map[string]any{"subtype": "initialize", "hooks": map[string]any{}, "sdkMcpServers": []any{}}})
	}
	if err != nil {
		return out, err
	}
	var models []agentview.ModelOption
	var text string
	var thread string
	total := 0
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 256<<10)
	for scanner.Scan() {
		raw := scanner.Bytes()
		total += len(raw)
		if total > 2<<20 {
			return out, fmt.Errorf("provider output limit exceeded")
		}
		var v struct {
			ID       json.RawMessage
			Method   string
			Params   json.RawMessage
			Result   json.RawMessage
			Error    json.RawMessage
			Type     string
			IsError  bool `json:"is_error"`
			Response struct {
				RequestID string `json:"request_id"`
				Subtype   string
				Response  json.RawMessage
			}
			Message struct{ Content []struct{ Type, Text string } }
			Usage   json.RawMessage
			Cost    json.RawMessage `json:"total_cost_usd"`
		}
		if json.Unmarshal(raw, &v) != nil {
			return out, fmt.Errorf("invalid auxiliary provider protocol")
		}
		if len(v.Error) > 0 && string(v.Error) != "null" {
			return out, fmt.Errorf("provider rejected auxiliary request %s; check login/model/effort", string(v.ID))
		}
		if in.Profile.Agent == "claude" {
			switch v.Type {
			case "control_response":
				if v.Response.RequestID != "init" {
					continue
				}
				if v.Response.Subtype != "success" {
					return out, fmt.Errorf("Claude initialization failed")
				}
				models = agentview.Models("claude", v.Response.Response)
				if in.Task == "models" {
					out.Models = models
					return out, nil
				}
				if e = ValidateModel(in.Profile, models); e != nil {
					return out, e
				}
				err = write(map[string]any{"type": "user", "session_id": "", "parent_tool_use_id": nil, "message": map[string]any{"role": "user", "content": "Task: " + in.Task + "\n" + in.Text}})
			case "control_request":
				return out, fmt.Errorf("auxiliary provider requested a tool or permission; denied")
			case "assistant":
				for _, b := range v.Message.Content {
					if b.Type == "tool_use" {
						return out, fmt.Errorf("auxiliary tool use denied")
					}
					if b.Type == "text" {
						text += b.Text
					}
				}
			case "result":
				if v.IsError {
					return out, fmt.Errorf("Claude auxiliary generation failed; check account/model")
				}
				var result string
				_ = json.Unmarshal(v.Result, &result)
				if result != "" {
					text = result
				}
				out, e = decodeOutput(text)
				out.Usage = v.Usage
				if len(v.Cost) > 0 {
					out.Usage, _ = json.Marshal(map[string]json.RawMessage{"tokens": v.Usage, "total_cost_usd": v.Cost})
				}
				if len(out.Usage) > 8192 {
					return Output{}, fmt.Errorf("provider usage payload too large")
				}
				return out, e
			}
		} else {
			if len(v.ID) > 0 && v.Method != "" {
				if v.Method != "account/chatgptAuthTokens/refresh" || token == nil {
					_ = write(map[string]any{"id": v.ID, "error": map[string]any{"code": -32601, "message": "auxiliary tools and permissions are disabled"}})
					return out, fmt.Errorf("auxiliary request denied: %s", v.Method)
				}
				var p struct {
					PreviousAccountID string `json:"previousAccountId"`
				}
				_ = json.Unmarshal(v.Params, &p)
				t, e := token(p.PreviousAccountID, true)
				if e != nil {
					return out, e
				}
				err = write(map[string]any{"id": v.ID, "result": t})
				continue
			}
			switch string(v.ID) {
			case `"init"`:
				if err = write(map[string]any{"method": "initialized"}); err != nil {
					return out, err
				}
				if token != nil {
					t, e := token("", false)
					if e != nil {
						return out, e
					}
					err = rpc("auth", "account/login/start", map[string]any{"type": "chatgptAuthTokens", "accessToken": t.AccessToken, "chatgptAccountId": t.AccountID})
				} else {
					err = rpc("models", "model/list", map[string]any{"limit": 100})
				}
			case `"auth"`:
				err = rpc("models", "model/list", map[string]any{"limit": 100})
			case `"models"`:
				models = append(models, agentview.Models("codex", v.Result)...)
				var page struct{ NextCursor string }
				_ = json.Unmarshal(v.Result, &page)
				if page.NextCursor != "" {
					if len(models) > 1000 {
						return out, fmt.Errorf("model catalog too large")
					}
					err = rpc("models", "model/list", map[string]any{"limit": 100, "cursor": page.NextCursor})
					break
				}
				if in.Task == "models" {
					out.Models = models
					return out, nil
				}
				if e = ValidateModel(in.Profile, models); e != nil {
					return out, e
				}
				config := map[string]any{"web_search": "disabled", "mcp_servers": map[string]any{}, "project_doc_max_bytes": 0, "tools.update_plan.enabled": false, "tools.experimental_request_user_input.enabled": false}
				for _, name := range []string{"shell_tool", "unified_exec", "apply_patch_freeform", "view_image", "image_generation", "apps", "connectors", "plugins", "multi_agent", "collab", "js_repl", "code_mode", "memories", "memory_tool", "tool_search", "tool_suggest", "skill_search", "hooks", "codex_hooks", "request_permissions_tool", "sleep_tool", "goals", "browser_use", "computer_use", "workspace_dependencies"} {
					config["features."+name] = false
				}
				config["features.skip_host_skill_discovery"] = true
				err = rpc("thread", "thread/start", map[string]any{"model": in.Profile.Model, "cwd": dir, "ephemeral": true, "approvalPolicy": "never", "sandbox": "read-only", "baseInstructions": Instructions, "config": config})
			case `"thread"`:
				var p struct{ Thread struct{ ID string } }
				_ = json.Unmarshal(v.Result, &p)
				thread = p.Thread.ID
				if thread == "" {
					return out, fmt.Errorf("missing auxiliary thread")
				}
				params := map[string]any{"threadId": thread, "input": []any{map[string]any{"type": "text", "text": "Task: " + in.Task + "\n" + in.Text}}, "outputSchema": map[string]any{"type": "object", "properties": map[string]any{"summary": map[string]string{"type": "string"}, "suggestion": map[string]string{"type": "string"}, "checkpoint": map[string]string{"type": "string"}}, "required": []string{"summary", "suggestion", "checkpoint"}, "additionalProperties": false}}
				if in.Profile.Effort != "" {
					params["effort"] = in.Profile.Effort
				}
				err = rpc("turn", "turn/start", params)
			}
			switch v.Method {
			case "item/started", "item/completed":
				var p struct{ Item struct{ Type, Text string } }
				_ = json.Unmarshal(v.Params, &p)
				switch p.Item.Type {
				case "agentMessage":
					if v.Method == "item/completed" {
						text = p.Item.Text
					}
				case "reasoning", "userMessage":
				default:
					return out, fmt.Errorf("unexpected auxiliary tool/item %q; execution stopped", p.Item.Type)
				}
			case "thread/tokenUsage/updated":
				var p struct {
					TokenUsage struct{ Last json.RawMessage }
				}
				_ = json.Unmarshal(v.Params, &p)
				out.Usage = p.TokenUsage.Last
			case "turn/completed":
				var p struct{ Turn struct{ Status string } }
				_ = json.Unmarshal(v.Params, &p)
				if p.Turn.Status != "completed" {
					return out, fmt.Errorf("Codex auxiliary turn did not complete")
				}
				usage := out.Usage
				out, e = decodeOutput(text)
				out.Usage = usage
				return out, e
			}
		}
		if err != nil {
			return out, err
		}
		if len(text) > MaxOutput {
			return out, fmt.Errorf("auxiliary response too large")
		}
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	return out, fmt.Errorf("auxiliary provider exited without a result")
}
func decodeOutput(text string) (Output, error) {
	var o Output
	if len(text) > MaxOutput {
		return o, fmt.Errorf("auxiliary response too large")
	}
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimSuffix(text, "```")
	if e := json.Unmarshal([]byte(text), &o); e != nil {
		return o, fmt.Errorf("auxiliary response was not valid structured output")
	}
	if len(o.Checkpoint) > CheckpointLimit || len(o.Summary) > SummaryLimit || len(o.Suggestion) > SuggestionLimit {
		return Output{}, fmt.Errorf("auxiliary response exceeded field limits")
	}
	o.Usage = nil
	o.Models = nil
	return o, nil
}
