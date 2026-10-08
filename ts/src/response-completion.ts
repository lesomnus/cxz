import { t } from "./i18n";
import type { SessionEvent } from "../gen/cxz/session_pb";
import { payload } from "./journal";

export type ResponseCompletion = {
  durationMs?: number;
  durationSource?: string;
  tokenScope?: string;
  metrics: Record<string, number>;
};
const valid = (value: unknown): value is number =>
  typeof value === "number" && Number.isFinite(value) && value >= 0;

// This map is bounded by the current history window, not all visited responses.
export function responseCompletions(events: SessionEvent[]) {
  return responseCompletionIndex(events).completions;
}

// Use the same association for the response footer and transcript projection:
// only completion records represented by a loaded final response are redundant.
export function responseCompletionIndex(events: SessionEvent[]) {
  const result = new Map<string, ResponseCompletion>();
  const representedEnds = new Set<bigint>();
  const candidates = new Map<string, { last?: SessionEvent; start?: bigint }>();
  const bySeq = new Map(events.map((e) => [e.seq.toString(), e]));
  for (const e of events) {
    const state = candidates.get(e.runId) ?? {};
    candidates.set(e.runId, state);
    if (e.kind === "input" && state.start === undefined) state.start = e.timeMs;
    if (e.kind === "assistant") {
      state.last = e;
      // Historical projections attach a successful completion to its original
      // response row, while live streams retain the native turn_end record.
      if (
        e.response?.completionJson.length &&
        !["commentary", "subagent"].includes(e.response.phase)
      ) {
        try {
          const c = JSON.parse(
            new TextDecoder().decode(e.response.completionJson),
          );
          if (String(c.response_seq) === e.seq.toString()) {
            candidates.delete(e.runId);
            result.set(e.seq.toString(), {
              durationMs: valid(c.duration_ms) ? c.duration_ms : undefined,
              durationSource: c.duration_source,
              tokenScope: c.token_scope,
              metrics: Object.fromEntries(
                Object.entries(c.metrics ?? {}).filter(([, v]) => valid(v)),
              ) as Record<string, number>,
            });
          }
        } catch {
          /* Retain the answer if its optional completion is malformed. */
        }
      }
    }
    if (e.kind !== "turn_end") continue;
    const last = state.last,
      start = state.start;
    candidates.delete(e.runId);
    if (e.text !== "completed") continue;
    if (e.response?.completionJson.length) {
      try {
        const c = JSON.parse(
          new TextDecoder().decode(e.response.completionJson),
        );
        const target = bySeq.get(String(c.response_seq));
        if (
          !target ||
          target.kind !== "assistant" ||
          target.runId !== e.runId ||
          target.seq >= e.seq ||
          ["commentary", "subagent"].includes(target.response?.phase ?? "")
        )
          continue;
        const metrics = Object.fromEntries(
          Object.entries(c.metrics ?? {}).filter(([, v]) => valid(v)),
        );
        result.set(target.seq.toString(), {
          durationMs: valid(c.duration_ms) ? c.duration_ms : undefined,
          durationSource: c.duration_source,
          tokenScope: c.token_scope,
          metrics: metrics as Record<string, number>,
        });
        representedEnds.add(e.seq);
      } catch {
        /* A malformed snapshot is not evidence of a final answer. */
      }
      continue;
    }
    // Legacy events retain native turn_end payloads. Never promote explicit commentary.
    const phase = last?.response?.phase || (last && payload(last).item?.phase);
    if (!last || (phase && phase !== "final_answer")) continue;
    const p = payload(e);
    const duration = p.duration_ms ?? p.turn?.durationMs;
    const metrics: Record<string, number> = {};
    for (const [native, common] of Object.entries({
      input_tokens: "input_tokens",
      output_tokens: "output_tokens",
      cache_read_input_tokens: "cache_read_tokens",
      cache_creation_input_tokens: "cache_write_tokens",
    })) {
      if (valid(p.usage?.[native])) metrics[common] = p.usage[native];
    }
    for (const [native, common] of Object.entries({
      total_cost_usd: "cost_usd",
      duration_api_ms: "api_duration_ms",
      num_turns: "api_turns",
    })) {
      if (valid(p[native])) metrics[common] = p[native];
    }
    const elapsed =
      start !== undefined && start > 0n && e.timeMs >= start
        ? Number(e.timeMs - start)
        : undefined;
    result.set(last.seq.toString(), {
      durationMs: valid(duration) ? duration : elapsed,
      durationSource: valid(duration) ? "provider" : "cxz",
      tokenScope: "turn",
      metrics,
    });
    representedEnds.add(e.seq);
  }
  return { completions: result, representedEnds };
}

export function durationLabel(ms: number) {
  if (ms < 1000) return `${Math.round(ms)}ms`;
  const seconds = ms / 1000;
  const rounded = Math.round(seconds);
  return seconds < 60
    ? t("{seconds}s", { seconds: seconds.toFixed(1) })
    : t("{minutes}m {seconds}s", {
        minutes: Math.floor(rounded / 60),
        seconds: rounded % 60,
      });
}
