package auxiliary

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/accounts"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAuxiliaryProtocolProcess(t *testing.T) {
	mode := os.Getenv("CXZ_AUX_PROTOCOL_TEST")
	if mode == "" {
		return
	}
	send := func(v any) { _ = json.NewEncoder(os.Stdout).Encode(v) }
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		var q struct {
			ID     string
			Method string
			Params map[string]any
			Type   string
		}
		_ = json.Unmarshal(s.Bytes(), &q)
		if strings.HasPrefix(mode, "claude") {
			if q.Type == "control_request" {
				send(map[string]any{"type": "control_response", "response": map[string]any{"request_id": "init", "subtype": "success", "response": map[string]any{"models": []any{map[string]any{"value": "test", "supportedEffortLevels": []string{"low"}}}}}})
			} else if q.Type == "user" {
				if mode == "claude-tool" {
					send(map[string]any{"type": "control_request"})
					continue
				}
				send(map[string]any{"type": "result", "result": `{"summary":"요약","suggestion":"다음 작업","checkpoint":""}`, "usage": map[string]int{"input_tokens": 4}})
			}
			continue
		}
		result := any(map[string]any{})
		switch q.Method {
		case "initialize":
		case "model/list":
			result = map[string]any{"data": []any{map[string]any{"model": "test", "supportedReasoningEfforts": []any{map[string]any{"reasoningEffort": "low"}}}}}
		case "thread/start":
			cfg, _ := q.Params["config"].(map[string]any)
			if q.Params["ephemeral"] != true || q.Params["approvalPolicy"] != "never" || cfg["features.shell_tool"] != false || cfg["web_search"] != "disabled" {
				os.Exit(8)
			}
			result = map[string]any{"thread": map[string]string{"id": "thread"}}
		case "turn/start":
			if mode == "codex-tool" {
				send(map[string]any{"method": "item/started", "params": map[string]any{"item": map[string]string{"type": "commandExecution"}}})
				continue
			}
			if mode == "codex-hang" {
				time.Sleep(time.Minute)
				continue
			}
			send(map[string]any{"method": "item/completed", "params": map[string]any{"item": map[string]string{"type": "agentMessage", "text": `{"summary":"요약","suggestion":"다음 작업","checkpoint":""}`}}})
			send(map[string]any{"method": "thread/tokenUsage/updated", "params": map[string]any{"tokenUsage": map[string]any{"last": map[string]int{"input_tokens": 4}}}})
			send(map[string]any{"method": "turn/completed", "params": map[string]any{"turn": map[string]string{"status": "completed"}}})
		}
		if q.ID != "" {
			send(map[string]any{"id": q.ID, "result": result})
		}
	}
	os.Exit(0)
}
func TestProviderProtocolsAndDenials(t *testing.T) {
	for _, mode := range []string{"codex", "claude", "codex-tool", "claude-tool", "codex-hang"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			p := profile()
			p.Agent = strings.Split(mode, "-")[0]
			out, e := Provider(ctx, os.Args[0], t.TempDir(), accounts.LaunchAuth{Args: []string{"-test.run=^TestAuxiliaryProtocolProcess$", "--"}, Env: append(os.Environ(), "CXZ_AUX_PROTOCOL_TEST="+mode)}, Input{Profile: p, Task: "combined", Text: "test input"}, nil)
			if strings.Contains(mode, "-") {
				if e == nil {
					t.Fatal("expected denial/timeout")
				}
				return
			}
			if e != nil || out.Summary != "요약" || out.Suggestion != "다음 작업" || len(out.Usage) == 0 {
				t.Fatal(fmt.Sprintf("%+v %v", out, e))
			}
		})
	}
}
