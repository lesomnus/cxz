import { t } from "../../../shared/i18n/i18n";
import type { SessionEvent } from "../../../../gen/cxz/session_pb";
// Bound cached records; the transcript mounts only its visible rows.
export const MAX_EVENTS = 8192;
const internalEventKinds = new Set([
  "raw",
  "state",
  "vendor",
  "intent",
  "receipt",
  "approval_resolved",
  "models",
  "models_status",
  "usage",
  "usage_status",
  "history_checkpoint",
]);

// The journal also carries protocol bytes and control/telemetry snapshots.
// Keep them for replay and metadata, but do not allocate transcript rows for them.
export function isTranscriptEvent(e: SessionEvent) {
  return !internalEventKinds.has(e.kind);
}

export function mergeEvents(
  previous: SessionEvent[],
  incoming: SessionEvent[],
  edge: "older" | "newer" = "newer",
) {
  const map = new Map(previous.map((e) => [e.seq, e]));
  for (const e of incoming) map.set(e.seq, e);
  let sorted = [...map.values()].sort((a, b) =>
    a.seq < b.seq ? -1 : a.seq > b.seq ? 1 : 0,
  );
  if (sorted.length > MAX_EVENTS) {
    // Token/protocol traffic can fill whole pages without adding a single row.
    // Prefer readable records and tool approval resolutions when trimming, while
    // retaining both fetched boundaries for pagination and stream replay.
    sorted = sorted.filter(
      (e, index) =>
        index === 0 ||
        index === sorted.length - 1 ||
        isTranscriptEvent(e) ||
        e.kind === "approval_resolved",
    );
  }
  return edge === "older"
    ? sorted.slice(0, MAX_EVENTS)
    : sorted.slice(-MAX_EVENTS);
}
export function payload(e: SessionEvent): Record<string, any> {
  try {
    return JSON.parse(new TextDecoder().decode(e.payload)) ?? {};
  } catch {
    return {};
  }
}
export function detail(e: SessionEvent) {
  try {
    return JSON.stringify(payload(e), null, 2);
  } catch {
    return "";
  }
}
export type Question = {
  key: string;
  header?: string;
  text: string;
  multi: boolean;
  other: boolean;
  secret: boolean;
  options: { label: string; description?: string; preview?: string }[];
};
export function questions(agent: string, e: SessionEvent): Question[] {
  const p = payload(e);
  const async = agent === "codex" && e.text === "agentMessage/questions";
  let wire: unknown;
  if (async) {
    if (
      p.item?.type !== "agentMessage" ||
      p.item?.delivery !== "async" ||
      !p.item?.id
    )
      return [];
    wire = p.item.questions;
  } else if (agent === "claude" && e.text === "AskUserQuestion")
    wire = p.input?.questions;
  else if (agent === "codex" && e.text === "item/tool/requestUserInput")
    wire = p.params?.questions;
  else return [];
  if (!Array.isArray(wire) || !wire.length) return [];
  const seen = new Set<string>();
  const result: Question[] = [];
  for (const [index, q] of wire.entries()) {
    if (!q || typeof q !== "object") return [];
    const key = async ? String(index) : agent === "claude" ? q.question : q.id;
    const text = async ? q.title : q.question;
    if (
      typeof key !== "string" ||
      !key.trim() ||
      typeof text !== "string" ||
      !text.trim() ||
      seen.has(key)
    )
      return [];
    seen.add(key);
    const choices = q.options ?? [];
    if (!Array.isArray(choices)) return [];
    const labels = new Set<string>();
    const options: Question["options"] = [];
    for (const option of choices) {
      const o = async ? { label: option } : option;
      if (
        !o ||
        typeof o.label !== "string" ||
        !o.label.trim() ||
        labels.has(o.label)
      )
        return [];
      labels.add(o.label);
      options.push({
        label: o.label,
        ...(typeof o.description === "string"
          ? { description: o.description }
          : {}),
        ...(typeof o.preview === "string" ? { preview: o.preview } : {}),
      });
    }
    result.push({
      key,
      text,
      ...(typeof q.header === "string" ? { header: q.header } : {}),
      multi: agent === "claude" && !!q.multiSelect,
      other: async || agent === "claude" || !!q.isOther || options.length === 0,
      secret: !!q.isSecret,
      options,
    });
  }
  return result;
}
export function approvalTitle(e: SessionEvent) {
  const p = payload(e);
  return e.text === "mcpServer/elicitation/request"
    ? `MCP · ${p.params?.serverName ?? t("Unknown server")}`
    : e.text;
}

export function pendingAfter(
  pending: SessionEvent[],
  events: SessionEvent[],
  after = 0n,
) {
  let out = pending;
  for (const e of events) {
    if (e.seq <= after) continue;
    if (e.kind === "approval" || e.kind === "approval_resolved") {
      out = out.filter(
        (p) => p.runId !== e.runId || p.requestId !== e.requestId,
      );
      if (e.kind === "approval") out = [...out, e];
    }
  }
  return out;
}
