package telemetry

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/telemetryauth"
)

var versions sync.Map
var versionPattern = regexp.MustCompile(`\b(\d+)\.(\d+)\.(\d+)`)

// NativeUsage is deliberately conservative. Unknown/old binaries retain the
// existing Supervisor parser instead of silently producing empty usage.
func NativeUsage(kind, version string) bool {
	p := versionPattern.FindStringSubmatch(version)
	if p == nil {
		return false
	}
	major, _ := strconv.Atoi(p[1])
	minor, _ := strconv.Atoi(p[2])
	if kind == "claude" {
		return major >= 2
	}
	return kind == "codex" && (major > 0 || minor >= 164)
}
func nativeBinary(kind, bin string) bool {
	st, err := os.Stat(bin)
	if err != nil {
		return false
	}
	key := bin + ":" + strconv.FormatInt(st.ModTime().UnixNano(), 10) + ":" + strconv.FormatInt(st.Size(), 10)
	if v, ok := versions.Load(key); ok {
		return v.(bool)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	v := err == nil && NativeUsage(kind, string(out))
	versions.Store(key, v)
	return v
}
func environment(env []string, values map[string]string) []string {
	out := make([]string, 0, len(env)+len(values))
	for _, v := range env {
		k, _, _ := strings.Cut(v, "=")
		if _, replace := values[k]; !replace {
			out = append(out, v)
		}
	}
	for k, v := range values {
		out = append(out, k+"="+v)
	}
	return out
}

func NativeLaunch(agent, endpoint, token string, args, env []string) ([]string, []string) {
	if agent == "claude" {
		env = environment(env, map[string]string{
			"CLAUDE_CODE_ENABLE_TELEMETRY": "1", "OTEL_METRICS_EXPORTER": "otlp",
			"OTEL_EXPORTER_OTLP_METRICS_PROTOCOL": "http/protobuf", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": endpoint,
			"OTEL_EXPORTER_OTLP_METRICS_HEADERS": "Authorization=Bearer%20" + token,
			"OTEL_METRIC_EXPORT_INTERVAL":        "10000", "OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE": "delta",
			"OTEL_LOGS_EXPORTER": "none", "OTEL_TRACES_EXPORTER": "none",
			"OTEL_LOG_USER_PROMPTS": "0",
		})
	} else if agent == "codex" {
		args = append(args, "-c", `otel.metrics_exporter={otlp-http={endpoint=`+strconv.Quote(endpoint)+`,protocol="binary",headers={Authorization=`+strconv.Quote("Bearer "+token)+`}}}`,
			"-c", `otel.exporter="none"`, "-c", `otel.log_user_prompt=false`)
	}
	return args, env
}

// Prepare reads only the runtime's own capability. Availability is checked once
// per agent process; a delayed native export never changes the selected source.
func Prepare(root string, session core.Session, run string, args, env []string) (*Publisher, []string, []string) {
	if _, err := os.Stat(filepath.Join(filepath.Dir(telemetryauth.Socket), "enabled")); err != nil {
		return nil, args, env
	}
	var runtime struct{ ProjectID, Token string }
	b, err := os.ReadFile(filepath.Join(filepath.Dir(root), "runtime.json"))
	if err != nil || json.Unmarshal(b, &runtime) != nil || runtime.Token == "" {
		return nil, args, env
	}
	identity := telemetryauth.Identity{Project: runtime.ProjectID, Session: session.ID, Account: session.Account, Agent: session.Kind, Run: run, Source: "supervisor", UsageSource: "supervisor", Scope: "conversation"}
	if nativeBinary(session.Kind, session.Agent) {
		identity.UsageSource = "native"
	}
	p, err := NewPublisher(telemetryauth.Socket, runtime.Token, identity)
	if err != nil {
		return nil, args, env
	}
	if identity.UsageSource == "native" {
		identity.Source = "native"
		token, err := telemetryauth.Sign(runtime.Token, identity)
		if err == nil {
			args, env = NativeLaunch(session.Kind, RelayEndpoint, token, args, env)
		}
	}
	return p, args, env
}
