package supervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// Opt-in capability probe: a real CLI with an empty profile, no user prompt,
// credentials or model invocation. Logs field names only, never response data.
func TestInstalledClaudeQuotaControl(t *testing.T) {
	if os.Getenv("CXZ_PROBE_CLAUDE") != "1" {
		t.Skip("set CXZ_PROBE_CLAUDE=1 for isolated CLI capability probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "claude", claudeArgs()...)
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
			if err := encoder.Encode(map[string]any{"type": "control_request", "request_id": "quota", "request": map[string]any{"subtype": "get_usage", "skip_behaviors": true}}); err != nil {
				t.Fatal(err)
			}
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
