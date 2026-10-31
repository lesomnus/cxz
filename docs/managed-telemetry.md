# Managed usage telemetry

An installed Manager manages one OpenTelemetry Collector and one ClickHouse
server. Both run in separate, installation-owned containers. Updating or
restarting either service does not stop project runtimes or agents. A local
source runtime without the packaged Collector keeps the existing history-based
statistics path.

The server image includes `cxz-otelcol`, built with OCB 0.164.0, core 0.164.0 /
stable modules 1.70.0, and contrib 0.164.1. The distribution includes only the
OTLP receiver, memory limiter, cxz identity processor, ClickHouse exporter,
persistent file storage, health check, and file/environment configuration
providers. Its module versions and checksums are locked separately from cxz.
The first uncached image build compiles these dependencies; subsequent image
builds reuse the Collector stage until its sources or locks change.

## Data path

```text
Claude / Codex native OTLP/HTTP metrics
    -> project runtime on 127.0.0.1:7347
    -> OTLP/gRPC over /cxz/tools/telemetry/collector.sock
    -> installation Collector -> ClickHouse

Supervisor committed events -> normalized OTLP metrics -> same Unix socket
Manager auxiliary-task results -> normalized OTLP metrics -> same Unix socket
```

Projects already mount the shared tools directory. The socket's containing
directory is mounted, so replacing Collector does not require remounting or
recreating every project. Non-root agents can connect through a read-only tools
mount. ClickHouse credentials and the ingest authorization registry are not in
that shared tools directory. ClickHouse's ports are not published on the host.
The Collector health endpoint is on the installation-private Docker network.

A project-scoped HMAC capability signs the session, account, agent, run, scope
and selected usage source. Collector checks the current private registry before
accepting a batch and replaces all cxz resource attributes. It strips unknown
resource/point attributes, including provider user identity. Auxiliary work has
its own short-lived registration and account while retaining the parent
project/session attribution. Projects cannot submit usage for another project.
No prompt, answer, tool output, provider transcript, OTLP logs or traces is
collected. Only metric values and allowlisted labels are stored.

## Native metrics and fallback

At process launch, known Claude 2.x and Codex 0.164+ binaries receive native
metrics configuration. Older or unrecognized versions use the Supervisor's
existing JSON/RPC usage parser. The choice is fixed for that process: delayed
native metrics never cause fallback tokens to be counted a second time.
Supervisor still emits conversation and turn counters with native collection.
Auxiliary tasks use their returned usage and actual selected account, without
adding another export path inside their isolated helper.

For unsupported providers, normalize their committed usage into the common
`usagemetrics` contract rather than reading private provider transcript files.
No extra agent process or model request is needed to collect usage.

Stream/event metadata is summed until turn/state boundaries, avoiding a metric
series and database request for each streamed text fragment. Native exporters
flush periodically; Claude is configured for ten seconds. Values can therefore
arrive after the visible response finishes. Native cumulative counters are
differenced by run, resource, attributes and start time. Delta counters are
summed. Retries with identical point identity/timestamp are deduplicated on
query. Codex input includes cached tokens; statistics subtract them and ignore
overlapping total/reasoning subsets. Missing cost remains unknown. Reported
amounts are provider estimates, not account billing or a model-price estimate.

## Settings, retention and outages

On the Linux installation host, edit `settings.jsonc`:

```jsonc
"telemetry": {
  "enabled": true,
  "retention_days": 90,
  "queue_mib": 64
}
```

These are the defaults; zero selects the default duration/queue size. Retention
accepts 1–3660 days and the queue 1–1024 MiB. Apply changes with
`cxz install --recreate`. Automatic Manager updates preserve the environment
configuration. A manual installation/recreation reads the host settings again.
Disabling stops the owned telemetry containers and keeps their data volume.
Re-enabling reuses it. A server/runtime update is required; updating web assets
alone does not install telemetry.

The Manager brings up the stack in the background and retries failed setup.
Images are fetched and ownership is checked before replacing a service. Agents
already running when this feature is installed receive launch instrumentation
on their next normal start; no active work is terminated to enable telemetry.
Existing conversations are not backfilled. Collection begins with instrumented
runs and successful auxiliary tasks after installation.

Display-history trimming does not remove analytics. Raw metric tables have the
configured ClickHouse TTL; expiry runs during background merges. Queries also
enforce the retention boundary. This is a rolling retention window, not an
unlimited lifetime ledger or a hard disk-space quota.

The Supervisor buffer is limited to 128 numeric batches. Export runs outside
its event lock, and a final flush waits at most two seconds. Collector's durable
queue is limited to the configured byte count; its storage file has a separate
2x cap (at least 16 MiB for small queues) and online/startup compaction. Both services have Docker memory limits
(Collector 256 MiB, ClickHouse 1 GiB) and rotated Docker logs. ClickHouse system
query/trace/metric logs are disabled. During a prolonged outage, full queues
reject/drop further metrics; agent execution continues. This is best effort
collection and statistics always report `complete=false` rather than asserting
that every token was captured.

Permanent session purge/project erasure revokes ingestion and writes a durable exclusion before
physical metric cleanup. Queued late exports remain excluded and periodic
cleanup removes their rows. Exclusions expire after retention plus seven days;
queries already exclude metrics older than retention. Erasure may return an
error for retry if its analytics cleanup cannot reach the database.

## Query and development

The existing typed `SessionService.ConversationStats` RPC queries ClickHouse on
managed installations, including account/project/session scopes and UTC daily
values. See [the API contract](conversation-stats.md). Existing clients need no
new transport. There is no ClickHouse endpoint or credential in browser/TUI
configuration, and account-quota graphs continue using provider quota APIs.

```sh
bash scripts/build-collector.sh /tmp/cxz-collector
CXZ_TELEMETRY_DOCKER=1 CXZ_COLLECTOR_BINARY=/tmp/cxz-collector/cxz-otelcol \
  go test -count=1 -run '^TestManagedTelemetryDocker$' -v ./internal/telemetrystack
```

The integration test creates and removes its own containers, network and
volumes. It checks the real Collector/ClickHouse schema, authentication, a
read-only non-root project's UDS connection, native sums/histograms, cumulative
baselines, retry deduplication, durable queue recovery and erasure/restart
behavior. CI runs this separately when telemetry-related sources change.

ClickHouse DDL is derived from the pinned Apache-2.0
[ClickHouse exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.164.1/exporter/clickhouseexporter),
with `DateTime64(9)` columns to preserve sub-second identity. Its license is
included alongside the schema sources. An exporter/schema upgrade must update
the pinned sources and schema version together and pass the integration test.
