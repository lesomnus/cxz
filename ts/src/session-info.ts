import type { Session, SessionEvent } from "../gen/cxz/session_pb";
import { payload } from "./journal";

type ObjectValue = Record<string, unknown>;
const object = (v: unknown): ObjectValue =>
  v !== null && typeof v === "object" && !Array.isArray(v)
    ? (v as ObjectValue)
    : {};
const text = (v: unknown) => (typeof v === "string" ? v : "");
const count = (v: unknown) =>
  typeof v === "number" && Number.isFinite(v) && v >= 0 ? v : undefined;
const resetTime = (v: unknown) => {
  const n = typeof v === "string" ? Date.parse(v) : (count(v) ?? 0) * 1000;
  return Number.isFinite(n) && n > 0 ? n : undefined;
};
type Quota = { remaining?: number; reset?: number; label: string };
export type SessionInfo = {
  model: string;
  effort: string;
  remaining?: number;
  reset?: number;
  quotaLabel?: string;
  contextUsed?: number;
  contextWindow?: number;
};

// Read provider snapshots, never infer account quota from cumulative token usage.
export function sessionInfo(
  s: Session | undefined,
  events: SessionEvent[],
): SessionInfo {
  const info: SessionInfo = { model: s?.model ?? "", effort: "" };
  const quotas = new Map<string, Quota>();
  let context: ObjectValue = {};
  let limits: ObjectValue = {};
  const addQuota = (
    key: string,
    label: string,
    value: unknown,
    field: string,
    multiplier = 1,
  ) => {
    const v = object(value);
    if (!Object.keys(v).length) return;
    const used = count(v[field]);
    quotas.set(key, {
      label,
      remaining:
        used === undefined
          ? undefined
          : Math.max(0, Math.min(100, 100 - used * multiplier)),
      reset: resetTime(v.resetsAt ?? v.resets_at),
    });
  };
  for (const e of events) {
    const currentRun = !e.runId || e.runId === s?.status?.runId;
    if (!["models", "usage", "turn_end", "compact"].includes(e.kind)) continue;
    const p = object(payload(e));
    if (e.kind === "models" && currentRun) {
      info.model =
        text(p.effective_model) || text(p.model) || s?.model || "default";
      // An explicit empty effective_effort means the provider did not report it.
      info.effort =
        "effective_effort" in p ? text(p.effective_effort) : text(p.effort);
      if (
        !("effective_effort" in p) &&
        !info.effort &&
        Array.isArray(p.models)
      ) {
        const selected = p.models
          .map(object)
          .find(
            (m) =>
              m.id === info.model ||
              m.resolved_id === info.model ||
              (info.model === "default" && m.default),
          );
        info.effort = text(selected?.default_effort);
      }
    }
    if (currentRun && e.kind === "compact") context = {};
    if (
      currentRun &&
      e.kind === "turn_end" &&
      Object.keys(object(p.modelUsage)).length
    )
      limits = object(p.modelUsage);
    if (e.kind !== "usage") continue;
    if (
      currentRun &&
      (e.text === "thread/tokenUsage/updated" || e.text === "context/message")
    )
      context = p;
    if (s?.agent === "codex" && e.text === "account/rateLimits/updated") {
      const buckets = object(p.rateLimitsByLimitId);
      const single = object(p.rateLimits);
      if (Object.keys(buckets).length) quotas.clear();
      else {
        const id = text(single.limitId) || "codex";
        for (const key of quotas.keys())
          if (key.startsWith(`${id}/`)) quotas.delete(key);
        buckets[id] = single;
      }
      for (const [id, value] of Object.entries(buckets)) {
        const bucket = object(value);
        for (const period of ["primary", "secondary"]) {
          const window = object(bucket[period]);
          const mins = count(window.windowDurationMins);
          const label =
            mins === undefined
              ? period
              : mins === 10080
                ? "weekly"
                : mins % 60 === 0
                  ? `${mins / 60}h`
                  : `${mins}m`;
          addQuota(
            `${id}/${period}`,
            `${text(bucket.limitName) || id} · ${label}`,
            window,
            "usedPercent",
          );
        }
      }
    }
    if (s?.agent === "claude" && e.text === "get_usage") {
      quotas.clear();
      for (const [key, window] of Object.entries(object(p.rate_limits)))
        addQuota(key, key.replaceAll("_", " "), window, "utilization");
    }
    if (s?.agent === "claude" && e.text === "rate_limit_event") {
      const window = object(p.rate_limit_info);
      const key = text(window.rateLimitType) || "quota";
      addQuota(key, key.replaceAll("_", " "), window, "utilization", 100);
    }
  }
  if (s?.agent === "codex") {
    const usage = object(context.tokenUsage);
    info.contextUsed = count(object(usage.last).totalTokens);
    info.contextWindow = count(usage.modelContextWindow);
  } else if (s?.agent === "claude") {
    const usage = object(context.usage);
    const input = count(usage.input_tokens);
    const read = count(usage.cache_read_input_tokens ?? 0);
    const write = count(usage.cache_creation_input_tokens ?? 0);
    if (input !== undefined && read !== undefined && write !== undefined)
      info.contextUsed = input + read + write;
    const model = text(context.model);
    let window = count(object(limits[model]).contextWindow);
    if (window === undefined && model) {
      const matches = Object.values(limits)
        .map(object)
        .filter((v) => v.canonicalModel === model);
      const windows = new Set(
        matches
          .map((v) => count(v.contextWindow))
          .filter((v) => v !== undefined),
      );
      if (windows.size === 1) window = [...windows][0];
    }
    info.contextWindow = window;
  }
  if (!info.contextWindow) info.contextWindow = undefined;
  const bucket = /spark/i.test(info.model) ? "spark" : "codex";
  const candidates = [...quotas.entries()]
    .filter(([key]) => s?.agent !== "codex" || key.startsWith(`${bucket}/`))
    .map(([, value]) => value)
    .sort((a, b) => (a.remaining ?? Infinity) - (b.remaining ?? Infinity));
  const quota = candidates[0];
  info.remaining = quota?.remaining;
  info.reset = quota?.reset;
  info.quotaLabel = quota?.label;
  return info;
}

export function formatTokens(n: number | undefined) {
  if (n === undefined) return "—";
  return new Intl.NumberFormat("en", {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(n);
}
export function formatReset(ms: number | undefined) {
  return ms === undefined
    ? "—"
    : new Date(ms).toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      });
}

export function quotaDots(percent: number | undefined, cells = 8) {
  if (percent === undefined) return "—";
  const levels = ["⣀", "⣄", "⣤", "⣶", "⣿"];
  const filled = Math.round(
    (Math.max(0, Math.min(100, percent)) * cells * 4) / 100,
  );
  return Array.from(
    { length: cells },
    (_, i) => levels[Math.max(0, Math.min(4, filled - i * 4))],
  ).join("");
}
