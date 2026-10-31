// Package telemetrystack manages installation-owned telemetry services. It never
// stops, recreates or sends control commands to project containers/Supervisors.
package telemetrystack

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/telemetryconfig"
	"github.com/lesomnus/cxz/internal/telemetrystore"
)

const ClickHouseImage = "clickhouse/clickhouse-server:26.10.3.7-alpine@sha256:1b09f476eb1109522cfc0e8e4ddf6a4305263e7cdd310f7bd1d3fd29dfcb8b33"

type Stack struct {
	Root, Owner, StateVolume, ToolsVolume, Image, Manager, ToolsRoot string
	Config                                                           telemetryconfig.Config
}

func (s Stack) prefix() string    { return "cxz-" + s.Owner[:12] + "-telemetry" }
func (s Stack) Endpoint() string  { return "http://" + s.prefix() + "-db:8123" }
func (s Stack) directory() string { return filepath.Join(s.Root, "telemetry") }
func (s Stack) tools() string {
	if s.ToolsRoot != "" {
		return s.ToolsRoot
	}
	return "/cxz/tools"
}
func (s Stack) Client() (*telemetrystore.Client, error) {
	password, err := os.ReadFile(filepath.Join(s.directory(), "db", "password"))
	if err != nil {
		return nil, err
	}
	client := telemetrystore.New(s.Endpoint(), string(password))
	client.RetentionDays = s.Config.Days()
	return client, nil
}
func (s Stack) validate() error {
	if len(s.Owner) < 12 || s.StateVolume == "" || s.ToolsVolume == "" || s.Image == "" || s.Manager == "" {
		return fmt.Errorf("telemetry requires an installed Manager with a shared state volume")
	}
	return s.Config.Validate()
}

func (s Stack) Prepare() error {
	if err := s.validate(); err != nil {
		return err
	}
	for _, dir := range []string{s.directory(), filepath.Join(s.directory(), "auth"), filepath.Join(s.directory(), "queue"), filepath.Join(s.directory(), "db"), filepath.Join(s.tools(), "telemetry")} {
		mode := os.FileMode(0700)
		if filepath.Base(dir) == "db" || dir == filepath.Join(s.tools(), "telemetry") {
			mode = 0755
		}
		if err := os.MkdirAll(dir, mode); err != nil {
			return err
		}
	}
	passwordFile := filepath.Join(s.directory(), "db", "password")
	password, err := os.ReadFile(passwordFile)
	if os.IsNotExist(err) {
		b := make([]byte, 32)
		if _, err = rand.Read(b); err != nil {
			return err
		}
		password = []byte(hex.EncodeToString(b))
		err = os.WriteFile(passwordFile, password, 0644)
	}
	if err != nil {
		return err
	}
	if len(password) != 64 {
		return fmt.Errorf("invalid telemetry database credential")
	}
	if err = os.WriteFile(filepath.Join(s.directory(), "db", "cxz.xml"), []byte(databaseConfig), 0644); err != nil {
		return err
	}
	if err = core.WriteJSON(filepath.Join(s.directory(), "preferences.json"), s.Config); err != nil {
		return err
	}
	if s.Config.Active() {
		return os.WriteFile(filepath.Join(s.tools(), "telemetry", "enabled"), []byte("1\n"), 0644)
	}
	return nil
}

func (s Stack) Ensure(ctx context.Context) error {
	if err := s.validate(); err != nil {
		return err
	}
	if !s.Config.Active() {
		return s.Stop(ctx)
	}
	if err := s.Prepare(); err != nil {
		return err
	}
	client, err := s.Client()
	if err != nil {
		return err
	}
	config := s.CollectorConfig(client.Password)
	if err = os.WriteFile(filepath.Join(s.directory(), "collector.yaml"), []byte(config), 0600); err != nil {
		return err
	}
	network := s.prefix()
	volume := s.prefix() + "-data"
	for _, item := range [][2]string{{"network", network}, {"volume", volume}} {
		if err = dockerx.EnsureResource(ctx, item[0], item[1], s.Owner, "telemetry"); err != nil {
			return err
		}
	}
	manager, err := dockerx.Inspect(ctx, s.Manager)
	if err != nil {
		return err
	}
	if manager.Config.Labels["cxz.owner"] != s.Owner {
		return fmt.Errorf("refusing telemetry network on an unowned Manager")
	}
	if _, ok := manager.NetworkSettings.Networks[network]; !ok {
		if _, err = dockerx.Run(ctx, "network", "connect", network, s.Manager); err != nil {
			return err
		}
	}
	common := []string{"--restart", "unless-stopped", "--network", network, "--label", "cxz.owner=" + s.Owner, "--label", "cxz.project=telemetry", "--log-driver", "json-file", "--log-opt", "max-size=10m", "--log-opt", "max-file=3"}
	dbargs := append(append([]string{}, common...), "--memory", "1g", "--pids-limit", "512", "--ulimit", "nofile=262144:262144",
		"--mount", "type=volume,source="+volume+",target=/var/lib/clickhouse",
		"--mount", "type=volume,source="+s.StateVolume+",volume-subpath=telemetry/db,target=/etc/clickhouse-server/config.d,readonly",
		"-e", "CLICKHOUSE_USER=cxz", "-e", "CLICKHOUSE_DB=telemetry", "--entrypoint", "/bin/sh")
	dbargs = append(dbargs, ClickHouseImage, "-ec", `export CLICKHOUSE_PASSWORD="$(cat /etc/clickhouse-server/config.d/password)"; exec /entrypoint.sh`)
	if err = s.ensureContainer(ctx, s.prefix()+"-db", ClickHouseImage, databaseConfig, dbargs); err != nil {
		return err
	}
	// ClickHouse needs to be ready before the exporter initializes its schema.
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if _, err = client.Query(ctx, "SELECT 1", nil); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("telemetry database did not become ready: %w", err)
		case <-tick.C:
		}
	}
	if err = s.initializeSchema(ctx, client); err != nil {
		return err
	}
	args := append(append([]string{}, common...), "--memory", "256m", "--pids-limit", "64", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--mount", "type=volume,source="+s.StateVolume+",volume-subpath=telemetry,target=/var/lib/cxz-telemetry",
		"--mount", "type=volume,source="+s.ToolsVolume+",volume-subpath=telemetry,target=/var/run/cxz-telemetry",
		"--entrypoint", "/bin/sh", s.Image, "-ec", `umask 000; rm -f /var/run/cxz-telemetry/collector.sock; exec /usr/local/bin/cxz-otelcol --config=/var/lib/cxz-telemetry/collector.yaml`)
	if err = s.ensureContainer(ctx, s.prefix()+"-collector", s.Image, config, args); err != nil {
		return err
	}
	healthCtx, healthDone := context.WithTimeout(ctx, 20*time.Second)
	defer healthDone()
	for {
		r, _ := http.NewRequestWithContext(healthCtx, http.MethodGet, "http://"+s.prefix()+"-collector:13133/", nil)
		response, err := client.HTTP.Do(r)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				break
			}
		}
		select {
		case <-healthCtx.Done():
			return fmt.Errorf("telemetry collector did not become healthy: %w", healthCtx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	return os.WriteFile(filepath.Join(s.tools(), "telemetry", "enabled"), []byte("1\n"), 0644)
}

func (s Stack) initializeSchema(ctx context.Context, client *telemetrystore.Client) error {
	type stamp struct{ Version, Days int }
	expected := stamp{1, s.Config.Days()}
	var existing stamp
	file := filepath.Join(s.directory(), "schema.json")
	if b, err := os.ReadFile(file); err == nil && json.Unmarshal(b, &existing) == nil && existing == expected {
		b, err := client.Query(ctx, `SELECT count() FROM system.tables WHERE database='telemetry' AND name IN ('otel_metrics_sum','otel_metrics_histogram','otel_metrics_gauge','otel_metrics_exp_histogram','otel_metrics_summary','collection','erased_sessions')`, nil)
		if err == nil && strings.TrimSpace(string(b)) == "7" {
			return nil
		}
	}
	if err := client.Initialize(ctx, s.Config.Days()); err != nil {
		return err
	}
	return core.WriteJSON(file, expected)
}

// Inspect labels before replacement; fetch the replacement image before removing
// a healthy service. A configuration/image change affects telemetry only.
func (s Stack) ensureContainer(ctx context.Context, name, image, config string, args []string) error {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(image+"\n"+config)))
	old, err := dockerx.Inspect(ctx, name)
	if err == nil {
		if old.Config.Labels["cxz.owner"] != s.Owner || old.Config.Labels["cxz.project"] != "telemetry" {
			return fmt.Errorf("refusing unowned telemetry container %s", name)
		}
		if old.Config.Image == image && old.Config.Labels["cxz.telemetry.config"] == hash {
			current, lookupErr := dockerx.Run(ctx, "image", "inspect", "--format", "{{.Id}}", image)
			if lookupErr == nil && strings.TrimSpace(string(current)) == old.Image {
				if !old.State.Running {
					_, err = dockerx.Run(ctx, "start", old.ID)
				}
				return err
			}
		}
	} else if !dockerx.Missing(err) {
		return err
	}
	if _, e := dockerx.Run(ctx, "image", "inspect", image); e != nil {
		if _, e = dockerx.Run(ctx, "pull", image); e != nil {
			return e
		}
	}
	if old.ID != "" {
		if old.State.Running {
			if _, err = dockerx.Run(ctx, "stop", "--time", "10", old.ID); err != nil {
				return err
			}
		}
		if _, err = dockerx.Run(ctx, "rm", old.ID); err != nil {
			return err
		}
	}
	_, err = dockerx.Run(ctx, append([]string{"run", "-d", "--name", name, "--label", "cxz.telemetry.config=" + hash}, args...)...)
	return err
}
func (s Stack) Stop(ctx context.Context) error {
	if err := s.validate(); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(s.tools(), "telemetry", "enabled"))
	for _, suffix := range []string{"-collector", "-db"} {
		v, err := dockerx.Inspect(ctx, s.prefix()+suffix)
		if dockerx.Missing(err) {
			continue
		}
		if err != nil {
			return err
		}
		if v.Config.Labels["cxz.owner"] != s.Owner || v.Config.Labels["cxz.project"] != "telemetry" {
			return fmt.Errorf("refusing unowned telemetry service")
		}
		if v.State.Running {
			if _, err = dockerx.Run(ctx, "stop", "--time", "10", v.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s Stack) CollectorConfig(password string) string {
	return fmt.Sprintf(`receivers:
  otlp:
    protocols:
      grpc:
        endpoint: /var/run/cxz-telemetry/collector.sock
        transport: unix
        include_metadata: true
        max_recv_msg_size_mib: 4
processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: 192
    spike_limit_mib: 32
  cxzidentity:
    registry_directory: /var/lib/cxz-telemetry/auth
exporters:
  clickhouse:
    endpoint: tcp://%s-db:9000?dial_timeout=5s
    username: cxz
    password: %q
    database: telemetry
    create_schema: false
    ttl: %dh
    async_insert: false
    compress: lz4
    timeout: 10s
    retry_on_failure:
      enabled: true
      initial_interval: 1s
      max_interval: 30s
      max_elapsed_time: 0s
    sending_queue:
      enabled: true
      storage: file_storage
      sizer: bytes
      queue_size: %d
      num_consumers: 1
      block_on_overflow: false
      batch:
        flush_timeout: 1s
        min_size: 4096
extensions:
  file_storage:
    directory: /var/lib/cxz-telemetry/queue
    max_size: %d
    fsync: true
    create_directory: true
    compaction:
      directory: /var/lib/cxz-telemetry/queue/compact
      on_start: true
      on_rebound: true
      rebound_needed_threshold_mib: %d
      rebound_trigger_threshold_mib: 1
  health_check:
    endpoint: 0.0.0.0:13133
service:
  extensions: [file_storage, health_check]
  telemetry:
    logs:
      level: warn
    metrics:
      level: none
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [memory_limiter, cxzidentity]
      exporters: [clickhouse]
`, s.prefix(), password, s.Config.Days()*24, s.Config.QueueBytes(), max(16<<20, s.Config.QueueBytes()*2), max(1, s.Config.QueueBytes()>>21))
}

var databaseConfig = `<clickhouse>
  <listen_host>0.0.0.0</listen_host>
  <logger><level>warning</level><console>true</console><log remove="1"/><errorlog remove="1"/></logger>
  <max_server_memory_usage>805306368</max_server_memory_usage>
  <max_connections>128</max_connections>
  <background_pool_size>16</background_pool_size>
  <background_move_pool_size>2</background_move_pool_size>
  <background_schedule_pool_size>4</background_schedule_pool_size>
  <background_message_broker_schedule_pool_size>2</background_message_broker_schedule_pool_size>
  <background_distributed_schedule_pool_size>2</background_distributed_schedule_pool_size>
  <mark_cache_size>33554432</mark_cache_size><uncompressed_cache_size>16777216</uncompressed_cache_size>
` + disableSystemLogs() + `</clickhouse>`

func disableSystemLogs() string {
	var b strings.Builder
	for _, name := range strings.Fields("query_log query_thread_log query_views_log query_metric_log part_log trace_log text_log metric_log asynchronous_metric_log asynchronous_insert_log processors_profile_log error_log crash_log session_log blob_storage_log backup_log zookeeper_log filesystem_cache_log") {
		fmt.Fprintf(&b, "<%s remove=\"1\"/>\n", name)
	}
	return b.String()
}
