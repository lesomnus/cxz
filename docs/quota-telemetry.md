# Account usage windows

The composer Usage graph shows provider-reported **remaining account quota**.
Its hover/focus panel lists every reported window separately, including session,
weekly and model-specific windows. Each row keeps its own reset time and color.
Unknown utilization remains unknown; it is not treated as zero usage.

## Current compact-graph selection

The graph currently chooses the window with the lowest known remaining
percentage. For Codex it first selects the `spark` bucket when the effective
model name contains `spark`, otherwise `codex`. Claude considers all reported
windows, including model-specific weekly limits. This is cxz's selection policy;
the parser does not use a provider-designated default-display flag.

The hover panel preserves the other windows instead of discarding them after
this selection. Codex bucket names distinguish multiple model groups. Window
labels use `session` for a reported five-hour window and `weekly` for seven days;
other durations and unknown window names retain their reported meaning.

## Codex app-server

`account/rateLimits/read` replies and `account/rateLimits/updated` notification
parameters share the following payload shape. These are illustrative values,
not an account snapshot:

```json
{
  "rateLimitsByLimitId": {
    "codex": {
      "limitId": "codex",
      "limitName": "Codex",
      "primary": {
        "usedPercent": 20,
        "windowDurationMins": 300,
        "resetsAt": 1900000000
      },
      "secondary": {
        "usedPercent": 70,
        "windowDurationMins": 10080,
        "resetsAt": 1900500000
      }
    }
  }
}
```

Older/single-bucket payloads provide that bucket as `rateLimits` instead.
`limitId` identifies its group; a missing ID falls back to `codex`. A nonempty
`rateLimitsByLimitId` snapshot replaces all buckets, while a single-bucket update
replaces only that bucket, including removing its omitted windows.

`usedPercent` is a percentage, so remaining is `100 - usedPercent`. `resetsAt`
is Unix time in **seconds**. `windowDurationMins` distinguishes the actual period:
300 is five hours, 10080 is seven days. The parser does not infer duration from
the words `primary` and `secondary` when duration is absent.

## Claude stream-json

cxz requests `get_usage` through the authenticated CLI's control channel. The
successful control response body has this shape:

```json
{
  "rate_limits_available": true,
  "rate_limits": {
    "five_hour": {
      "utilization": 20,
      "resets_at": "2030-01-01T00:00:00Z"
    },
    "seven_day": {
      "utilization": 60,
      "resets_at": "2030-01-06T00:00:00Z"
    },
    "seven_day_opus": null,
    "seven_day_sonnet": null,
    "model_scoped": [
      {
        "display_name": "Fable",
        "utilization": 65,
        "resets_at": "2030-01-06T00:00:00Z"
      }
    ]
  }
}
```

This response's `utilization` is a **percentage**. Other reported window keys
such as `seven_day_oauth_apps` are retained too. Named `model_scoped` entries use
their server-provided `display_name`; the UI does not invent a period for them.
Null windows are omitted; an
explicit unavailable/empty full snapshot clears previous quota information.

Claude can also push an individual `rate_limit_event`:

```json
{
  "type": "rate_limit_event",
  "rate_limit_info": {
    "rateLimitType": "five_hour",
    "utilization": 0.2,
    "resetsAt": 1900000000,
    "status": "allowed",
    "unifiedWindows": {
      "five_hour": { "utilization": 0.2, "resetsAt": 1900000000 },
      "seven_day": { "utilization": 0.6, "resetsAt": 1900500000 }
    }
  }
}
```

In this event, `utilization` is a **fraction**, so remaining is
`100 - utilization * 100`. Its `resetsAt` uses Unix seconds. An individual update
does not discard other windows. When `rate_limit_info.unifiedWindows` contains
per-window objects, the composer reads those too. An explicitly null entry
removes that window; absent utilization is still unknown.

The installed Claude Code 2.1.314 executable's control schemas were inspected
for `get_usage`, `model_scoped` and `unifiedWindows`. These are experimental CLI
interfaces, so unknown window keys remain visible rather than requiring a
fixed list of model names.

## Transport and implementation

The Supervisor stores these bodies as `usage` events. Codex strips the RPC
`params`/`result` envelope; Claude `get_usage` strips the control-response
envelope. Shared Claude snapshots may additionally carry `_cxz_observed_ms`,
which cxz adds as an observation timestamp. The period/remaining labels in the
UI are derived values rather than raw provider fields.

The browser retains the latest individual-period and unified-window
notifications independently, so other windows survive hundreds of context
updates and transcript window replacements. A later full unavailable snapshot
still clears those older values.

- `internal/supervisor/codex.go`: account quota polling and notifications.
- `internal/supervisor/claude_args.go`, `quota_shared.go`: Claude control query
  and shared snapshot handling.
- `ts/src/features/session/model/session-info.ts`: preserve windows and select
  the compact-graph value.
- `ts/src/features/session/composer/usage-info.tsx`: per-window hover rows.
- Storybook `Conversation/Usage`: `SessionAndWeekly`, `MultipleBuckets`, and
  `ClaudeWindows` exercise combined windows and unknown values.
