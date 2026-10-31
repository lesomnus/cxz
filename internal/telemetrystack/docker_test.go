package telemetrystack

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/resourceid"
	"github.com/lesomnus/cxz/internal/telemetry"
	"github.com/lesomnus/cxz/internal/telemetryauth"
	"github.com/lesomnus/cxz/internal/usagemetrics"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// Set CXZ_TELEMETRY_DOCKER=1 and CXZ_COLLECTOR_BINARY to an OCB-built executable.
// The test manager/volumes/network are isolated and removed even on failure.
func TestManagedTelemetryDocker(t *testing.T) {
	if os.Getenv("CXZ_TELEMETRY_TEST_CHILD") == "1" {
		testPipeline(t)
		return
	}
	if os.Getenv("CXZ_TELEMETRY_DOCKER") != "1" {
		t.Skip("set CXZ_TELEMETRY_DOCKER=1 for isolated ClickHouse/Collector integration")
	}
	collector := os.Getenv("CXZ_COLLECTOR_BINARY")
	if collector == "" {
		t.Fatal("CXZ_COLLECTOR_BINARY is required")
	}
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", args[0], err, b)
		}
		return string(b)
	}
	run("go", "test", "-c", "-o", filepath.Join(dir, "integration.test"), ".")
	b, err := os.ReadFile(collector)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "cxz-otelcol"), b, 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM docker:29-cli\nCOPY cxz-otelcol /usr/local/bin/cxz-otelcol\nCOPY integration.test /integration.test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	owner := core.ID()
	prefix := "cxz-" + owner[:12] + "-telemetry"
	image := "cxz-telemetry-test:" + owner
	manager := prefix + "-test"
	defer func() {
		for _, name := range []string{prefix + "-collector", prefix + "-db", manager} {
			_ = exec.Command("docker", "rm", "-f", name).Run()
		}
		_ = exec.Command("docker", "network", "rm", prefix).Run()
		for _, vol := range []string{prefix + "-data", prefix + "-state", prefix + "-tools"} {
			_ = exec.Command("docker", "volume", "rm", vol).Run()
		}
		_ = exec.Command("docker", "image", "rm", image).Run()
	}()
	run("docker", "build", "-q", "-t", image, dir)
	for _, vol := range []string{prefix + "-state", prefix + "-tools"} {
		run("docker", "volume", "create", "--label", "cxz.owner="+owner, vol)
	}
	args := []string{"docker", "run", "--rm", "--name", manager, "--label", "cxz.owner=" + owner,
		"-e", "CXZ_TELEMETRY_TEST_CHILD=1", "-e", "CXZ_TEST_OWNER=" + owner, "-e", "CXZ_TEST_IMAGE=" + image, "-e", "CXZ_TEST_MANAGER=" + manager,
		"--mount", "type=volume,source=" + prefix + "-state,target=/state", "--mount", "type=volume,source=" + prefix + "-tools,target=/tools"}
	if host := os.Getenv("DOCKER_HOST"); host != "" && !strings.HasPrefix(host, "unix://") {
		u, err := url.Parse(host)
		if err != nil {
			t.Fatal(err)
		}
		ips, err := net.LookupIP(u.Hostname())
		if err != nil {
			t.Fatal(err)
		}
		u.Host = net.JoinHostPort(ips[0].String(), u.Port())
		host = u.String()
		args = append(args, "-e", "DOCKER_HOST="+host)
	} else {
		args = append(args, "--mount", "type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock")
	}
	args = append(args, "--entrypoint", "/integration.test", image, "-test.run=^TestManagedTelemetryDocker$", "-test.v", "-test.timeout=180s")
	t.Log(run(args...))
}

func testPipeline(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Second)
	defer cancel()
	owner := os.Getenv("CXZ_TEST_OWNER")
	s := Stack{Root: "/state", Owner: owner, StateVolume: "cxz-" + owner[:12] + "-telemetry-state", ToolsVolume: "cxz-" + owner[:12] + "-telemetry-tools", Image: os.Getenv("CXZ_TEST_IMAGE"), Manager: os.Getenv("CXZ_TEST_MANAGER"), ToolsRoot: "/tools"}
	if err := s.Prepare(); err != nil {
		t.Fatal(err)
	}
	i := telemetryauth.Identity{Project: "00112233445566778899aabbccddeeff", Session: "fixture-session", Account: "fixture-account", Agent: "codex", Run: "fixture-run", Source: "supervisor", UsageSource: "supervisor", Scope: "conversation"}
	registry := telemetryauth.Registry{Secret: "test-project-capability", Sessions: map[string]telemetryauth.Session{i.Session: {Account: i.Account, Agent: i.Agent}}}
	if err := core.WriteJSON(filepath.Join(s.directory(), "auth", i.Project+".json"), registry); err != nil {
		t.Fatal(err)
	}
	if err := s.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	client, err := s.Client()
	if err != nil {
		t.Fatal(err)
	}
	socket := "/tools/telemetry/collector.sock"
	conn, err := grpc.NewClient("passthrough:///collector", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	exporter := collectorpb.NewMetricsServiceClient(conn)
	token, _ := telemetryauth.Sign(registry.Secret, i)
	exportCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	now := time.Now().Add(-time.Second).UnixMilli()
	event := core.Event{Seq: 1, TimeMS: now, Kind: "turn_end", RunID: i.Run, Payload: json.RawMessage(`{"usage":{"inputTokens":100,"cachedInputTokens":40,"outputTokens":20}}`)}
	state := usagemetrics.State{}
	batch := telemetry.EventMetrics(i, event, state.Event(event))
	if _, err = exporter.Export(exportCtx, batch, grpc.WaitForReady(true)); err != nil {
		t.Fatal(err)
	}
	// At-least-once delivery must not count the same point twice.
	if _, err = exporter.Export(exportCtx, batch); err != nil {
		t.Fatal(err)
	}
	account := hex.EncodeToString(resourceid.Bytes(9, i.Account))
	for {
		stats, err := client.Stats(ctx, "account", account, now-1000, now+1000)
		if err == nil && stats.GetTotal().GetInputTokens() == 60 {
			if stats.GetTotal().GetFinishedTurns() != 1 || stats.GetTotal().GetCacheReadTokens() != 40 || stats.GetTotal().GetOutputTokens() != 20 {
				t.Fatal("incorrect normalized/deduplicated usage", stats)
			}
			if stats.GetTotal().GetCostReportedTurns() != 0 || stats.GetComplete() {
				t.Fatal("unknown cost/coverage reported as known", stats)
			}
			break
		}
		select {
		case <-ctx.Done():
			logs, _ := exec.Command("docker", "logs", s.prefix()+"-collector").CombinedOutput()
			t.Fatalf("statistics: %v\n%s", err, logs)
		case <-time.After(200 * time.Millisecond):
		}
	}
	bad := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer forged")
	if _, err = exporter.Export(bad, batch); err == nil {
		t.Fatal("collector accepted forged capability")
	}
	testNativeMetrics(t, s, registry, now)
	// A Manager restart/configuration check must preserve both services/data.
	before := dockerID(t, s.prefix()+"-collector")
	if err = s.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	if after := dockerID(t, s.prefix()+"-collector"); after != before {
		t.Fatal("unchanged collector was replaced")
	}
	if err = client.Erase(ctx, hex.EncodeToString(resourceid.Bytes(8, i.Session))); err != nil {
		t.Fatal(err)
	}
	if _, err = exporter.Export(exportCtx, batch); err != nil {
		t.Fatal(err)
	} // simulate an already queued late export
	stats, err := client.Stats(ctx, "account", account, now-1000, now+1000)
	if err != nil || stats.GetTotal().GetInputTokens() != 0 {
		t.Fatal("erased session resurrected", stats, err)
	}
	for _, suffix := range []string{"-collector", "-db"} {
		if _, err = exec.Command("docker", "restart", s.prefix()+suffix).CombinedOutput(); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	stats, err = client.Stats(ctx, "account", account, now-1000, now+1000)
	if err != nil || stats.GetTotal().GetInputTokens() != 0 {
		t.Fatal("erasure lost across restart", stats, err)
	}
	if err = s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat("/tools/telemetry/enabled"); !os.IsNotExist(err) {
		t.Fatal("disabled marker retained", err)
	}
}

func testNativeMetrics(t *testing.T, s Stack, registry telemetryauth.Registry, now int64) {
	t.Helper()
	ctx := t.Context()
	file := filepath.Join(s.directory(), "auth", "00112233445566778899aabbccddeeff.json")
	i := telemetryauth.Identity{Project: "00112233445566778899aabbccddeeff", Session: "native-session", Account: "native-account", Agent: "claude", Run: "native-run", Source: "native", UsageSource: "native", Scope: "conversation"}
	registry.Sessions[i.Session] = telemetryauth.Session{Account: i.Account, Agent: i.Agent}
	if err := core.WriteJSON(file, registry); err != nil {
		t.Fatal(err)
	}
	// A project runtime's native HTTP relay uses the same UDS export path.
	relay, err := telemetry.StartRelay(ctx, "127.0.0.1:17347", "/tools/telemetry/collector.sock", i.Project, registry.Secret)
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	token, _ := telemetryauth.Sign(registry.Secret, i)
	batch := nativeBatch(i, []*metricspb.Metric{sumMetric("claude_code.token.usage", "input", now-5000, now-6000, 100, 2), sumMetric("claude_code.token.usage", "input", now-3000, now-6000, 130, 2), sumMetric("claude_code.cost.usage", "", now-5000, now-6000, 0.1, 2), sumMetric("claude_code.cost.usage", "", now-3000, now-6000, 0.3, 2)})
	post := func(token string, batch *collectorpb.ExportMetricsServiceRequest) {
		t.Helper()
		body, err := proto.Marshal(batch)
		if err != nil {
			t.Fatal(err)
		}
		r, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:17347/v1/metrics", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/x-protobuf")
		r.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatal(res.Status, string(b))
		}
	}
	post(token, batch)
	post(token, batch)
	client, err := s.Client()
	if err != nil {
		t.Fatal(err)
	}
	account := hex.EncodeToString(resourceid.Bytes(9, i.Account))
	waitStats(t, func() bool {
		stats, err := client.Stats(ctx, "account", account, now-4000, now-1000)
		if err != nil {
			t.Log(err)
			return false
		}
		return stats.GetTotal().GetInputTokens() == 30 && stats.GetTotal().GetReportedCostUsd() > 0.199 && stats.GetTotal().GetReportedCostUsd() < 0.201 && stats.GetTotal().GetCostReportedTurns() > 0
	})
	// Delta histograms preserve distinct sub-millisecond points, ignore total
	// and reasoning subsets, and exclude a separate native-source fallback.
	i.Session = "histogram-session"
	i.Agent = "codex"
	registry.Sessions[i.Session] = telemetryauth.Session{Account: i.Account, Agent: i.Agent}
	if err = core.WriteJSON(file, registry); err != nil {
		t.Fatal(err)
	}
	token, _ = telemetryauth.Sign(registry.Secret, i)
	var metrics []*metricspb.Metric
	for _, entry := range []struct {
		kind  string
		value float64
	}{{"input", 100}, {"cached_input", 40}, {"output", 20}, {"reasoning_output", 7}, {"total", 120}} {
		for n := 0; n < 2; n++ {
			metrics = append(metrics, &metricspb.Metric{Name: "codex.turn.token_usage", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, DataPoints: []*metricspb.HistogramDataPoint{{Attributes: []*commonpb.KeyValue{kv("token_type", entry.kind)}, TimeUnixNano: uint64(time.UnixMilli(now-2000).UnixNano()) + uint64(n), Count: 1, Sum: proto.Float64(entry.value), BucketCounts: []uint64{1}}}}}})
		}
	}
	post(token, nativeBatch(i, metrics))
	waitStats(t, func() bool {
		stats, err := client.Stats(ctx, "session", hex.EncodeToString(resourceid.Bytes(8, i.Session)), now-2500, now-1500)
		if err != nil {
			t.Log(err)
			return false
		}
		return stats.GetTotal().GetInputTokens() == 120 && stats.GetTotal().GetCacheReadTokens() == 80 && stats.GetTotal().GetOutputTokens() == 40
	})
	// Stats scope cannot be redirected by a native agent's cxz attributes.
	stats, err := client.Stats(ctx, "project", "ffffffffffffffffffffffffffffffff", now-10000, now+1000)
	if err != nil || stats.GetTotal().GetInputTokens() != 0 {
		t.Fatal("forged native resource escaped project", stats, err)
	}
}
func kv(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}
func nativeBatch(i telemetryauth.Identity, metrics []*metricspb.Metric) *collectorpb.ExportMetricsServiceRequest {
	return &collectorpb.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{kv("cxz.project.id", "ffffffffffffffffffffffffffffffff"), kv("user.email", "never-store@example.test")}}, ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: metrics}}}}}
}
func sumMetric(name, kind string, at, start int64, value float64, temporality metricspb.AggregationTemporality) *metricspb.Metric {
	return &metricspb.Metric{Name: name, Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{AggregationTemporality: temporality, IsMonotonic: true, DataPoints: []*metricspb.NumberDataPoint{{Attributes: []*commonpb.KeyValue{kv("type", kind)}, TimeUnixNano: uint64(time.UnixMilli(at).UnixNano()), StartTimeUnixNano: uint64(time.UnixMilli(start).UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: value}}}}}}
}
func waitStats(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if ready() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("statistics did not converge")
		}
		time.Sleep(200 * time.Millisecond)
	}
}
func dockerID(t *testing.T, name string) string {
	t.Helper()
	b, err := exec.Command("docker", "inspect", "--format", "{{.Id}}", name).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(b))
	}
	return strings.TrimSpace(string(bytes.TrimSpace(b)))
}
