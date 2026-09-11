# Claude Code verification

Companion to [codex-verification.md](codex-verification.md). Tested against
**Claude Code 2.1.267**, x86_64, Debian 13 devcontainer, 2026-09-11.

**Updated after executable verification:** host-routed approvals, denials,
questions, interrupts, and same-session resume all work. The earlier permission
probe omitted `--permission-prompt-tool stdio`. Claude Code remains the first
adapter; permission bypass is not needed for the MVP.

Probes ran in a scratch working directory with the *shared* config dir.
Credentials were deliberately **not** copied to an isolated `CLAUDE_CONFIG_DIR`:
the refresh token rotates (§4.2), and a second live copy would have invalidated
the session doing the testing. This is the hazard the design already documents,
met in practice.

## Confirmed

**Bidirectional stream-json works.** `claude -p --input-format stream-json
--output-format stream-json --verbose` accepts user messages on stdin and emits
events on stdout, exactly as §3.1's supervisor needs.

**A control channel exists and answers.** An SDK-style handshake

```json
{"type":"control_request","request_id":"init-1","request":{"subtype":"initialize","hooks":{}}}
```

drew a `control_response` carrying the session's command list. So the transport
is genuinely request/response in both directions, matching §3.3.

**Event schema:**

| Event | Contents |
|---|---|
| `system` / `init` | `session_id`, `cwd`, `tools`, model |
| `rate_limit_event` | `utilization`, `resetsAt`, `rateLimitType`, and `unifiedWindows.{five_hour,seven_day}` |
| `assistant` | a whole message; `content` blocks are `text` or `tool_use` |
| `user` | `tool_result` blocks, with `is_error` |
| `result` | `duration_api_ms`, `stop_reason`, `total_cost_usd`, `usage` with cache-read/creation breakdown |

`--include-partial-messages` adds streaming chunks; `--replay-user-messages`
echoes stdin back for acknowledgement.

**Rate limits arrive in-band.** cld calls a separate usage API for this; in
protocol mode both the five-hour and seven-day windows are pushed as events.
Same for cost: `result.total_cost_usd` replaces the OTLP receiver cld built.

## Earlier failed probe: missing stdio permission routing

Three attempts, none of which produced a permission `control_request` — the
Bash tool simply ran:

1. `--permission-mode manual --permission-prompts host`
2. the same, plus `--settings '{"permissions":{"defaultMode":"manual","allow":[],"deny":[]}}'`
   to rule out inherited config (the shared config sets `defaultMode: auto`)
3. the same, preceded by the `initialize` control handshake above

The SDK implementation resolves the missing declaration: a `canUseTool`
callback makes the TypeScript SDK pass **`--permission-prompt-tool stdio`**.
The Python SDK does the same. `stdio` is a special routing target; no MCP
permission server or extra initialize capability is needed for this path.

Sources inspected:

- [Official TypeScript SDK package, 0.3.268](https://registry.npmjs.org/@anthropic-ai/claude-agent-sdk/-/claude-agent-sdk-0.3.268.tgz),
  `sdk.mjs`: process launch and control request/response handlers. The probe
  drives the installed **CLI 2.1.267**, not a CLI bundled with that SDK.
- [Python SDK permission configuration at the inspected commit](https://github.com/anthropics/claude-agent-sdk-python/blob/3379406f18fcea64617d25663d811dfdde8cd171/src/claude_agent_sdk/types.py),
  `_configure_can_use_tool`.
- [Official permission documentation](https://code.claude.com/docs/en/agent-sdk/permissions)
  and [user input documentation](https://code.claude.com/docs/en/agent-sdk/user-input).

## Reproducible protocol probe

[Probe source](../../scripts/probes/claude-protocol.mjs), requiring Node.js and
an already authenticated Claude CLI. It uses scratch files, no SDK runtime,
no subagents, no copied credentials, and no changes to shared settings.
Normal conversations use the existing credential directory in place; only the
login URL probe uses an empty isolated config directory. Normal CLI session
history and usage are still produced.

```sh
node scripts/probes/claude-protocol.mjs /tmp/cxz-claude-results
node scripts/probes/claude-protocol.mjs /tmp/cxz-claude-baseline --without-prompt-tool
node scripts/probes/claude-protocol.mjs /tmp/cxz-claude-login --login-only
```

Use a fresh output directory per run. The first two commands make model calls;
the last only starts and closes an OAuth flow without completing login.
`CXZ_CLAUDE_BIN` selects the installed executable. The observed model was
`claude-opus-5[1m]`; the probe uses the CLI's selected model rather than pinning
a model alias. Model responses remain a source of integration-test variability.

Relevant launch settings:

```text
-p --input-format stream-json --output-format stream-json --verbose
--permission-mode manual --permission-prompts host
--permission-prompt-tool stdio
--setting-sources=
--settings '{"permissions":{"defaultMode":"manual","allow":[],"deny":[],"ask":["Bash"]},"disableAllHooks":true}'
```

Explicit `ask: ["Bash"]` makes even harmless file commands reach the permission
path. User/project settings, hooks and MCP servers are excluded by the full
probe. Otherwise an existing allow rule or auto mode can resolve a tool before
the host is asked. This means the original auto-execution observation alone
cannot establish which inherited rule allowed the command.

The controlled baseline uses the same configuration but omits only
`--permission-prompt-tool stdio`: no host request arrives and no file is created.
With the option present the request arrives and blocks execution until answered.

### Measured results

| Check | Result and evidence |
|---|---|
| Initialize | Correlated `control_response`; includes `current_permission_mode`, `session_state`, commands and account metadata |
| Approval blocks | Held response for **3,504 ms**; marker file absent and no terminal result throughout |
| Allow | `behavior: allow` plus `updatedInput` runs the command; expected file bytes verified |
| Deny | `behavior: deny` plus message prevents the file and produces an error `tool_result`; conversation continues |
| Multiple turns | Same process accepts successive user messages and retains `session_id` |
| User question | `AskUserQuestion` arrives as `can_use_tool`; returning `updatedInput.answers` carries the selected answer into the tool result |
| Interrupt | After approving a `sleep 20 && printf ...` command, `interrupt` returns an acknowledgment and terminal result within **26 ms** in this run |
| No delayed write | Marker still absent **22 seconds after interruption**, beyond the original sleep duration |
| Resume | Close stdin and wait for process exit; new process with `--resume=<session-id>` retains the ID and recalls a random marker from a previous turn without being given that marker again |
| Login initiation | In an empty config directory, `claude_authenticate` returns `manualUrl` and `automaticUrl`; no existing credentials copied or replaced |

Machine-readable evidence:

- [Successful suite](../../testdata/protocol/claude/verified/summary.json)
  and [sanitized events](../../testdata/protocol/claude/verified/events.jsonl).
- [No-stdio baseline](../../testdata/protocol/claude/no-stdio/summary.json).
- [Isolated login initiation](../../testdata/protocol/claude/login/summary.json).
- [Version and artifact manifest](../../testdata/protocol/claude/manifest.json).

Fixtures are parsed protocol records with relative times, pseudonymized IDs and
paths, and redacted account/OAuth information. They are not byte-identical raw
journals. The real supervisor must preserve vendor bytes separately from this
shareable test-fixture representation.

### Wire contract

The CLI requests permission on stdout:

```json
{"type":"control_request","request_id":"req-1","request":{"subtype":"can_use_tool","tool_name":"Bash","tool_use_id":"tool-1","input":{"command":"..."}}}
```

The host answers on stdin and keeps stdin open between turns:

```json
{"type":"control_response","response":{"subtype":"success","request_id":"req-1","response":{"behavior":"allow","updatedInput":{"command":"..."}}}}
```

Reject with `{"behavior":"deny","message":"..."}` in the inner response.
`request_id` correlates the control exchange; `tool_use_id` correlates the tool
call/result. Neither is a cxz session ID. The initialized protocol also accepts
`{"subtype":"interrupt"}` as a host-originated control request.

### Adapter consequences

- **Claude stays first.** Approvals and questions do not require a terminal or
  relaxed permissions. Optional capabilities still matter, but the earlier
  assumption that Claude cannot route them is disproven.
- **Treat interruption separately from failure.** This version returns
  `subtype: error_during_execution`, `is_error: true`,
  `terminal_reason: aborted_tools` after the requested interrupt. Use local
  interrupt intent and vendor terminal information together.
- **Do not trust `subtype: success` alone.** An exploratory run got a model
  refusal with `subtype: success`, `is_error: true`, `stop_reason: refusal`,
  `terminal_reason: api_error`. A terminal result must be classified by all
  relevant fields. The benign file-operation suite then passed on a fresh run.
- **State reporting is version-dependent.** This CLI emitted `system/status`
  with `status: requesting` and `system/init` for successive turns. The SDK
  0.3.268 type declarations also contain `system/session_state_changed`
  (`idle | running | requires_action`), but that event was **not observed** in
  CLI 2.1.267. Do not claim the vendor has no state surface; derive fallback
  state from submitted input, pending requests and results for this version.
- **Login has a control surface.** SDK implementation contains
  `claude_authenticate`, `claude_oauth_callback` (`authorizationCode`, `state`),
  and `claude_oauth_wait_for_completion`. Only URL initiation was exercised;
  existence of the other handlers does not prove completed login.

## Container recreation — verified with a shared Docker engine

The follow-up [Docker probe](../../scripts/probes/claude-container-recreate.mjs)
passed all three recovery scenarios on 2026-09-11, using CLI 2.1.267 and Docker
29.7.2. [Summary](../../testdata/protocol/claude/container-recreate-verified/summary.json)
and [sanitized events](../../testdata/protocol/claude/container-recreate-verified/events.jsonl)
record the container generations, checks and cleanup.

```sh
node scripts/probes/claude-container-recreate.mjs /tmp/cxz-claude-recreate-results
```

The script needs an existing local `golang:1.26-trixie` image, the native Claude
binary and an already-valid access token in the current credential store.
`CXZ_PROBE_IMAGE` and `CXZ_CLAUDE_BIN` override the image and executable. It
resolves the image tag to an immutable ID before starting. Each run has unique
container/volume names and ownership labels. Cleanup verifies those labels;
there is no prune, image deletion or use of other projects' resources.

No bind mounts are needed: three named volumes hold `/workspace`, `/cxz/state`
and the CLI binary. This avoids interpreting local `/workspace` as a Docker-host
path. For future bind tests, the verified shared project path is
`/workspaces/github.com/lesomnus/cxz`. The engine is outside this devcontainer.

Only the current short-lived access token is read into memory and sent over
`docker exec` stdin to the CLI process. It is absent from Docker container
environment metadata, command arguments and saved fixtures. The refresh token,
credential file and existing config are never copied. Consequently this verifies
conversation persistence, **not OAuth credential persistence**. Each new CLI
process receives the access token again.

| State at container removal | Observed recovery |
|---|---|
| Completed turn, CLI still alive | Remove container without closing CLI; create a different container with the same volumes. Workspace file persists, container-only `/tmp` marker disappears, and `--resume` preserves session ID and recalls the earlier random project codename. |
| Approval pending | Remove container before replying. Resume recalls the codename and the old marker file remains absent. A new tool request gets a different request ID. Sending the old approval while the new request waits does not execute either command or complete the turn during a 3.5-second observation. Denying the new request finishes normally. |
| Shell command running | Approve a command that writes `started`, sleeps 20 seconds, then writes a completion file. Remove the container after observing the first write. Resume preserves the session/context and partial file. After 22.1 seconds from the removal sequence, the completion file remains absent and the partial file still contains exactly one `started`. |

Resume prompts explicitly cancel the unfinished operation and ask only for
prior context; they do not resubmit the original command. This is evidence that
the CLI can recover this way, not a guarantee that arbitrary follow-up prompts
will never cause the model to retry work. The stale-reply test establishes
non-execution, not an explicit CLI rejection acknowledgment. cxz must still bind
requests to a container generation/run and reject stale input itself.

All three vendor transcript files were present on the state volume at the end,
and no `.credentials.json` had been created there. The run removed its five
containers (including provisioning) and three volumes after saving the evidence.
Synthetic workspace data was discarded with the test volumes.

**Provisioning finding:** the first run reached a tool permission error writing
the workspace. The successful run uses `volume-nocopy` mounts, initializes volume
ownership to UID/GID 1000, and checks ownership and writability in every new
container before starting Claude. The initial
[failure record](../../testdata/protocol/claude/container-recreate/summary.json)
is retained. Product provisioning must verify filesystem permissions in addition
to successfully starting an authenticated agent.

**Scope:** containers run as UID/GID 1000 at the same workspace path, with all
capabilities dropped and no Docker socket or privileged mode. The provisioning
helper temporarily has CHOWN capability to initialize the volumes. This is
direct Docker plus the vendor protocol. It does not yet exercise `devcontainer up`,
a cxz supervisor, a browser journal/projection, host-loss restore, or path moves.

## Remaining verification

- Complete browser OAuth, submit its callback, verify credential persistence
  and a subsequent authenticated turn. Requires the operator's browser sign-in;
  the URL-only probe intentionally does not perform this identity step.
- End-to-end cxz/devcontainer recreation, host restore, workspace relocation and
  UID changes. Direct Docker replacement, including an active tool and pending
  approval, now passes; the actual product lifecycle still needs implementation.
- Interrupt while an approval is pending, cancellation notifications, duplicate
  replies, input delivery ambiguity and reconnect replay. Stale replies across
  container replacement did not authorize a fresh request in the measured case;
  cxz-side generation checks remain unimplemented.
- Images and other attachments, CLI-version compatibility, and state events in
  the next pinned version. No hosted browser UI has been implemented yet.
- `apiKeyHelper` with an unchanged base URL, the §4.2 experiment.
