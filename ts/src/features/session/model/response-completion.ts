import { t } from "#src/shared/i18n/i18n.ts";
import type { SessionEvent } from "#gen/cxz/session_pb";
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
/**
 * A completed turn's metrics live on the response row the runtime folded them
 * onto, keyed by the sequence it names. Nothing is derived here: the
 * projection decided which response a turn belongs to, whether its phase makes
 * it eligible, and where the duration came from.
 *
 * This used to fold a native `turn_end` record itself, because a live stream
 * carried records while a page carried rows -- so the same conversation
 * arrived in two shapes and this had a branch for each. It arrives as rows
 * now. See cxz#155.
 */
export function responseCompletions(rows: SessionEvent[]) {
  const result = new Map<string, ResponseCompletion>();
  for (const e of rows) {
    const completion = e.response?.completionJson;
    if (!completion?.length) continue;
    try {
      const c = JSON.parse(new TextDecoder().decode(completion));
      // The row names the response the metrics belong to. A row carrying
      // somebody else's sequence is not this row's completion.
      if (String(c.response_seq) !== e.seq.toString()) continue;
      result.set(e.seq.toString(), {
        durationMs: valid(c.duration_ms) ? c.duration_ms : undefined,
        durationSource: c.duration_source,
        tokenScope: c.token_scope,
        metrics: Object.fromEntries(
          Object.entries(c.metrics ?? {}).filter(([, v]) => valid(v)),
        ) as Record<string, number>,
      });
    } catch {
      /* Retain the answer if its optional completion is malformed. */
    }
  }
  return result;
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
