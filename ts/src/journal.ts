import { t } from "./i18n";
import type { SessionEvent } from "../gen/cxz/session_pb";
// Bound cached records; the transcript mounts only its visible rows.
export const MAX_EVENTS = 512;
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
  text: string;
  multi: boolean;
  other: boolean;
  secret: boolean;
  options: { label: string; description?: string }[];
};
export function questions(agent: string, e: SessionEvent): Question[] {
  const p = payload(e);
  if (agent === "claude" && e.text === "AskUserQuestion")
    return (p.input?.questions ?? []).map((q: any) => ({
      key: q.question,
      text: q.question,
      multi: !!q.multiSelect,
      other: true,
      secret: !!q.isSecret,
      options: q.options ?? [],
    }));
  if (agent === "codex" && e.text === "item/tool/requestUserInput")
    return (p.params?.questions ?? []).map((q: any) => ({
      key: q.id,
      text: q.question,
      multi: false,
      other: !!q.isOther || !q.options?.length,
      secret: !!q.isSecret,
      options: q.options ?? [],
    }));
  return [];
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
