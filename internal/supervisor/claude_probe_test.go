package supervisor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/agentview"
)

// Opt-in capability probe: a real CLI with an empty profile, no user prompt,
// credentials or model invocation. Logs field names only, never response data.
func TestInstalledClaudeQuotaControl(t *testing.T) {
	if os.Getenv("CXZ_PROBE_CLAUDE") != "1" {
		t.Skip("set CXZ_PROBE_CLAUDE=1 for isolated CLI capability probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "claude", claudeRunArgs("", "max", "")...)
	cmd.Dir = t.TempDir()
	for _, v := range os.Environ() {
		key, _, _ := strings.Cut(v, "=")
		if strings.HasPrefix(key, "CLAUDE") || strings.HasPrefix(key, "ANTHROPIC") || strings.HasPrefix(key, "AWS_") || strings.HasPrefix(key, "GOOGLE_") || strings.HasPrefix(key, "OPENAI_") {
			continue
		}
		cmd.Env = append(cmd.Env, v)
	}
	cmd.Env = append(cmd.Env, "CLAUDE_CONFIG_DIR="+t.TempDir())
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { in.Close(); cancel(); cmd.Wait() }()
	encoder := json.NewEncoder(in)
	if err := encoder.Encode(map[string]any{"type": "control_request", "request_id": "initialize", "request": map[string]any{"subtype": "initialize", "hooks": map[string]any{}, "sdkMcpServers": []any{}}}); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var modelRequest map[string]any
	for scanner.Scan() {
		var v struct {
			Type     string `json:"type"`
			Response struct {
				RequestID string                     `json:"request_id"`
				Subtype   string                     `json:"subtype"`
				Error     string                     `json:"error"`
				Response  map[string]json.RawMessage `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal(scanner.Bytes(), &v) != nil || v.Type != "control_response" {
			continue
		}
		if v.Response.RequestID == "initialize" {
			if v.Response.Subtype != "success" {
				t.Fatal("CLI initialization rejected")
			}
			raw, _ := json.Marshal(v.Response.Response)
			models := agentview.Models("claude", raw)
			t.Logf("initialization catalog models=%d", len(models))
			modelRequest = map[string]any{"subtype": "set_model", "model": "default"}
			for _, model := range models {
				if slices.Contains(model.Efforts, "low") {
					modelRequest["model"] = model.ID
					break
				}
			}
			if err := encoder.Encode(map[string]any{"type": "control_request", "request_id": "restored-effort", "request": map[string]any{"subtype": "get_settings"}}); err != nil {
				t.Fatal(err)
			}
		} else if v.Response.RequestID == "restored-effort" {
			var applied struct{ Effort string }
			if json.Unmarshal(v.Response.Response["applied"], &applied) != nil || applied.Effort != "max" {
				t.Fatal("launch flag did not restore session-only max effort")
			}
			_ = encoder.Encode(map[string]any{"type": "control_request", "request_id": "model", "request": modelRequest})
		} else if v.Response.RequestID == "model" {
			if v.Response.Subtype != "success" {
				t.Fatal("set_model rejected")
			}
			_ = encoder.Encode(map[string]any{"type": "control_request", "request_id": "effort", "request": map[string]any{"subtype": "apply_flag_settings", "settings": map[string]any{"effortLevel": "low"}}})
		} else if v.Response.RequestID == "effort" {
			if v.Response.Subtype != "success" {
				t.Fatal("effort update rejected")
			}
			_ = encoder.Encode(map[string]any{"type": "control_request", "request_id": "settings", "request": map[string]any{"subtype": "get_settings"}})
		} else if v.Response.RequestID == "settings" {
			if v.Response.Subtype != "success" {
				t.Fatal("get_settings rejected")
			}
			raw, _ := json.Marshal(v.Response.Response)
			if !bytes.Contains(raw, []byte(`"effortLevel":"low"`)) {
				t.Fatal("effort not reflected in settings")
			}
			var applied struct{ Effort string }
			if json.Unmarshal(v.Response.Response["applied"], &applied) != nil || applied.Effort != "low" {
				t.Fatal("low effort was acknowledged without being applied")
			}
			_ = encoder.Encode(map[string]any{"type": "control_request", "request_id": "max-effort", "request": map[string]any{"subtype": "apply_flag_settings", "settings": map[string]any{"effortLevel": "max"}}})
		} else if v.Response.RequestID == "max-effort" {
			if v.Response.Subtype != "success" {
				t.Fatal("max effort rejected")
			}
			_ = encoder.Encode(map[string]any{"type": "control_request", "request_id": "max-settings", "request": map[string]any{"subtype": "get_settings"}})
		} else if v.Response.RequestID == "max-settings" {
			var applied struct{ Effort string }
			if json.Unmarshal(v.Response.Response["applied"], &applied) != nil || applied.Effort != "max" {
				t.Fatal("session-only max effort was not applied")
			}
			_ = encoder.Encode(map[string]any{"type": "control_request", "request_id": "reset-effort", "request": map[string]any{"subtype": "apply_flag_settings", "settings": map[string]any{"effortLevel": nil}}})
		} else if v.Response.RequestID == "reset-effort" {
			if v.Response.Subtype != "success" {
				t.Fatal("effort reset rejected")
			}
			_ = encoder.Encode(map[string]any{"type": "control_request", "request_id": "reset-settings", "request": map[string]any{"subtype": "get_settings"}})
		} else if v.Response.RequestID == "reset-settings" {
			var applied struct{ Effort string }
			if json.Unmarshal(v.Response.Response["applied"], &applied) != nil || applied.Effort == "" || applied.Effort == "max" {
				t.Fatal("effort reset did not restore the model default")
			}
			t.Log("max launch flag, low/max control, and default reset verified through get_settings.applied")
			_ = encoder.Encode(map[string]any{"type": "control_request", "request_id": "models", "request": map[string]any{"subtype": "list_models"}})
		} else if v.Response.RequestID == "models" {
			if v.Response.Subtype != "success" {
				t.Fatal("list_models rejected")
			}
			raw, _ := json.Marshal(v.Response.Response)
			models := agentview.Models("claude", raw)
			if len(models) == 0 {
				t.Fatal("list_models returned no parseable models")
			}
			t.Logf("live list_models catalog models=%d", len(models))
			_ = encoder.Encode(map[string]any{"type": "control_request", "request_id": "quota", "request": map[string]any{"subtype": "get_usage", "skip_behaviors": true}})
		} else if v.Response.RequestID == "quota" {
			if v.Response.Subtype != "success" {
				t.Logf("get_usage rejected; unsupported=%t", strings.Contains(strings.ToLower(v.Response.Error), "unsupported") || strings.Contains(strings.ToLower(v.Response.Error), "unknown"))
				return
			}
			keys := make([]string, 0, len(v.Response.Response))
			for key := range v.Response.Response {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			t.Logf("get_usage supported; response fields: %v", keys)
			var available bool
			if err := json.Unmarshal(v.Response.Response["rate_limits_available"], &available); err != nil {
				t.Fatal("quota-availability field is missing or changed type")
			}
			t.Logf("isolated profile rate_limits_available=%t", available)
			return
		}
	}
	t.Fatalf("CLI ended without quota response (timeout=%t, read error=%v)", ctx.Err() != nil, scanner.Err())
}
