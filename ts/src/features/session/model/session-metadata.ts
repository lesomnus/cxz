import type { SessionEvent } from "#gen/cxz/session_pb";
import { payload } from "./journal";

// Retain the latest capability/quota/context snapshots even after many token
// updates or transcript window replacements. Recent quota deltas preserve partial
// provider updates; distinguish full bucket snapshots from single-bucket updates.
export function mergeMetadata(
  previous: SessionEvent[],
  incoming: SessionEvent[],
) {
  const rows = incoming.filter(
    (e) =>
      ["models", "usage", "compact"].includes(e.kind) ||
      (e.kind === "turn_end" && payload(e).modelUsage),
  );
  if (!rows.length) return previous;
  const sorted = [
    ...new Map([...previous, ...rows].map((e) => [e.seq, e])).values(),
  ].sort((a, b) => (a.seq < b.seq ? -1 : a.seq > b.seq ? 1 : 0));
  const runs = [...new Set([...sorted].reverse().map((e) => e.runId))].slice(
    0,
    4,
  );
  const snapshots = new Map<string, SessionEvent>();
  for (const e of sorted) {
    if (!runs.includes(e.runId)) continue;
    const p = e.text === "account/rateLimits/updated" ? payload(e) : {};
    const bucket = p.rateLimitsByLimitId
      ? "all"
      : p.rateLimits?.limitId || "codex";
    const key = `${e.runId}/${e.kind}/${e.text}/${bucket}`;
    snapshots.set(key, e);
  }
  return [
    ...new Map(
      [...snapshots.values(), ...sorted.slice(-200)].map((e) => [e.seq, e]),
    ).values(),
  ].sort((a, b) => (a.seq < b.seq ? -1 : a.seq > b.seq ? 1 : 0));
}
