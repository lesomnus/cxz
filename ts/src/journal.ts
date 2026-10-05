import type { SessionEvent } from "../gen/cxz/session_pb";
// Four history pages in memory; the transcript mounts only its visible rows.
export const MAX_EVENTS = 512;
export function mergeEvents(
  previous: SessionEvent[],
  incoming: SessionEvent[],
  edge: "older" | "newer" = "newer",
) {
  const map = new Map(previous.map((e) => [e.seq, e]));
  for (const e of incoming) map.set(e.seq, e);
  const sorted = [...map.values()].sort((a, b) =>
    a.seq < b.seq ? -1 : a.seq > b.seq ? 1 : 0,
  );
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
    ? `MCP · ${p.params?.serverName ?? "Unknown server"}`
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
