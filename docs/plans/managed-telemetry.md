# Managed agent telemetry

Status: implementing in `feat/managed-agent-telemetry`.

The installation owns one ClickHouse and one minimal OCB Collector. Both are
independent of Manager and session processes. Collector components are pinned
and built in a separate module; server images include the resulting executable.
No host TCP ports or database credentials are exposed to projects.

## Delivery

1. Define authenticated telemetry identities and common usage normalization.
2. Build the minimal Collector: OTLP/gRPC UDS receiver, identity processor,
   memory limiter, ClickHouse exporter with bounded persistent queue, health
   check and file/env configuration. Pin OCB/core 0.164.0 and contrib 0.164.1.
3. Manage the two installation-owned containers, network, configuration and
   volumes; refresh Collector on image updates without restarting agents.
4. Add the project runtime loopback OTLP/HTTP relay. Forward protobuf/JSON over
   the tools volume's dedicated Unix socket; accept only authenticated session
   identities, bound request sizes/timeouts/concurrency, and tolerate outages.
5. Configure native Claude/Codex metrics on launch. Supervisor emits cxz turn
   metrics and normalized fallback usage for unsupported providers/versions.
   Choose usage source once per process; never add native and fallback tokens.
6. Query retained sum/histogram points through ConversationStats, normalize
   cumulative/delta streams and cache semantics, expose partial coverage, and
   preserve unknown cost/reporting distinctions. Retention is independent of
   conversation history and bounded queues never block agent progress.
7. Validate the actual collector/ClickHouse/UDS pipeline, authentication,
   duplicate exports, restarts, outages, schemas, install/update ownership,
   statistics and frontend compatibility. Document rollout and settings.

## Identity and isolation

Project capabilities already identify one project. A private per-project
registry under Manager's telemetry directory contains that capability and the
registered session/account/agent associations. Session-scoped telemetry tokens
are HMAC-authenticated with the project capability; the Collector verifies
both signature and registry membership and overwrites cxz resource attributes.
The registry is mounted only in Collector, never in agent containers. A tools
subdirectory exposes only the ingest socket. Mount the directory, not an inode,
so Collector recreation does not require project recreation.

Project runtime and Supervisor do not receive ClickHouse credentials. Alias
strings are resolved to the same canonical resource UUIDs as the resource API.
Native and adapter telemetry retain their source; missing counters stay unknown.

## Verification and rollout

Run focused Go tests/races, isolated Docker integration and server builds.
Checkpoint meaningful stages in commits. Existing user servers and agent
sessions are preserved. Existing running processes take instrumentation on their
next normal launch; no active work is interrupted to enable metrics.
