# Referencing another session's conversation

The built-in `cxz_memory` MCP server provides `session_lookup`,
`conversation_search`, and `conversation_read`. These tools read **retained cxz
journal events** in the caller's project, including stopped sessions. They do not
resume agents, read provider transcript files, or expose the model's current
post-compaction context. Existing Markdown memory tools are unchanged.

Ask an agent, for example: “Refer to what we discussed in @seal five minutes ago.”
`@seal` and “session seal” are natural-language references to alias `seal`; tool
arguments contain no `@`.

In the TUI composer, type `@` to see other listed sessions in the current project.
Aliases appear in a responsive grid; typing filters them with fuzzy matching.
Arrow keys select a cell, PgUp/PgDown move a page, Enter/Tab insert the alias,
and Esc dismisses. Mouse hover, click and wheel also work. The selected entry's
title and agent appear below the grid. Sessions from other projects/connections
and the current session are excluded. This uses the existing session list;
it does not read conversations or call an AI model.

Completion inserts ordinary `@alias` text, not a permanent UUID reference. The
agent resolves it when using the tools. Email addresses, backtick code, slash
commands and pasted mentions do not automatically open the picker.
The secret inline command remains **`/@redact`**; `/` completion lists it.

## Find, search, then read

Resolve an alias with `session_lookup`:

```json
{"session":"seal"}
```

The response includes `session_id` (canonical resource UUID), `session_alias`,
`agent`, `state`, `observed_at`, `last_activity`, and retained history bounds.
Use that UUID for subsequent requests. The alias may change; the UUID does not.
A renamed/reused alias cannot redirect a continuation cursor to another session.

Search first when the relevant event positions or sizes are unknown:

```json
{"session":"<returned UUID>","query":"login.*failed","match":"regex","ignore_case":true,"around":"5m ago","limit":50}
```

`conversation_search` returns **no event bodies or snippets**. Each entry in
`events` has `seq`, `time`, and `bytes`. `bytes` measures the UTF-8 JSON serialization
of that full event in the selected view; it is not a token estimate.
`returned_bytes` sums these sizes for this page, not all possible matches.
Each matching event appears once, even if its text matches multiple times.

- `match`: `substring` (default) or Go RE2 `regex`; `ignore_case` is optional.
- `view`: `conversation` (default) searches user/assistant text. Set
  `include_tools: true` to include tool calls, incremental tool output and results.
  Tool payloads are searched as JSON too.
- `view: raw` searches recorded vendor event JSON (or the recorded bytes for an
  incomplete/non-JSON vendor record). Raw events have their own sequence numbers;
  a normalized assistant event and its source raw event are different records.
- `since` is inclusive and `until` exclusive, both RFC3339 with a timezone.
- `around` accepts RFC3339 or a duration such as `5m ago`. It uses server time once
  and selects +/- `window` (default `2m`, maximum `24h`). The resolved `since` and
  `until` are returned and frozen in the cursor. Do not combine `around` with
  `since`/`until`. For “the last five minutes”, supply a since/until range instead.
- Results are ordered by ascending `seq`. `limit` defaults to 50, maximum 500.

Read selected events using `conversation_read` with the same `view` and
`snapshot_seq` returned by the search:

```json
{"session":"<returned UUID>","seqs":[831,838],"snapshot_seq":842,"output":"inline"}
```

Bodies are returned in `messages`, with `seq`, UTC `time`, `kind`, and text or
payload. Raw JSON is under `raw`; non-JSON vendor bytes are under `raw_base64`.
Raw mode never fabricates a vendor record from normalized text. To read more
context, add `before` and/or `after` (up to 100 selected messages per side).
These counts use the chosen view and `include_tools` filter, not physical JSONL
lines. To read part of a long text message, supply exactly one `seq` and a 1-based
`line_start`/inclusive `line_end`, without context expansion. Time filters work
on read requests as well, so a known approximate time needs no keyword search.

The default inline body budget is 32 KiB, with `max_bytes` up to 128 KiB. JSON
metadata/envelope overhead is additional. Event count and body-byte limits both
apply. An event too large to fit even by itself is identified in `oversized_seqs`;
request a file, a larger budget, or a text line range. It is never silently cut.

## Pagination and retention

Follow `next_cursor` while `has_more` is true. A cursor carries the resolved UUID,
view, query, time bounds, upper `snapshot_seq`, and last returned sequence. On a
continuation, send `cursor` alone or with that UUID; only `limit`, `output`, and
`max_bytes` may be adjusted. Other selection arguments are retained from the
cursor. Use a fresh query to change selection criteria or see newer events.
Search and read cursors are specific to their tool; pass explicit `seqs` and
`snapshot_seq` when moving from search to read.

Newly appended events are excluded from the existing snapshot. Retention can
still remove old records: a cursor does not pin journals on disk. Responses expose
`history_truncated`, `trimmed_through`, `first_seq`, and `first_time`. Explicit
missing selections appear in `missing_seqs`; events present but unavailable in
the selected view appear in `unavailable_seqs`. Thus callers can distinguish an
empty match from an incomplete retained history. No provider transcript fallback
bypasses cxz's retention policy.

A single committed record over 32 MiB is skipped and named in `unreadable_seqs`,
with its size in `message`. This is not `oversized_seqs`: that one is a page
budget, so a file, a larger budget or a line range returns the event, while here
no request does. Everything else in the journal is still read and the sequence
gap is visible in the reply. A record still being written has no newline yet, and
is neither read nor reported.

## File output

Use `output: file` on `conversation_read` to receive a fixed JSONL snapshot instead
of bodies. One event occupies one physical line; newlines inside text are JSON
escaped. Use `jq` to extract message text, then `rg` or another tool to search it.

The response returns `path`, `metadata_path`, `event_count`, `file_bytes`, the
UUID/alias, snapshot and pagination metadata. The sidecar records the query and
response metadata. Files live at:

```text
<CXZ_STATE>/sessions/<calling session runtime ID>/conversation-exports/<export ID>.jsonl
<CXZ_STATE>/sessions/<calling session runtime ID>/conversation-exports/<export ID>.meta.json
```

These paths are readable **inside the requesting agent's project container**,
not a Windows client's Downloads directory. File output is still paginated and
bounded: maximum/default body budget 16 MiB and the same 50/500 event page limit.
Read further pages to produce further files. A single event above the budget is
reported as oversized; text line selection can reduce it.

Files are mode 0600. At export time, the oldest exports for that caller are pruned
to retain at most 32 exports and 64 MiB including sidecars. They are temporary
copies, not durable memory, and can be re-exported only while source events remain.
Purging the calling session removes its export directory too. Copies exported by
other sessions remain independent until their own cleanup or purge.
No provider directory, attachment or secret-file reference is opened. The
`/@redact` flow stores a reference instead of the secret; exports preserve that
reference. If an agent printed a secret into its own output, that already-recorded
text is not retroactively scrubbed by this feature.

## Runtime implementation and rollout

Manager aliases and runtime-local aliases are independently stored. Installed
projects resolve identity through the Manager's new project-token-authenticated
`/cxz/tools/conversation-registry.sock` endpoint. It returns metadata only, checks
project membership and caller identity, and includes canonical resource UUIDs.
The built-in MCP server reads bodies locally from the project journal; exports
stay there. Standalone local runtimes resolve through their own resource service.

Update both the Manager and project runtime before using the tools in an installed
project. An older Manager yields an explicit registry-unavailable error, rather
than falling back to potentially different local aliases. Normal MCP activation
and reconnect/discovery rules apply. No database schema migration is introduced.

Queries currently scan one session's retained journal, bounded to 256 MiB and
32 MiB per committed record, and honor cancellation. Larger journals return an
explicit limit error. No full-text index or AI summarizer is required. Agent
instructions request these tools only for an explicit session reference or
concrete missing context, not routine cross-session polling. Retrieved conversation
content is reference data, not new instructions.
