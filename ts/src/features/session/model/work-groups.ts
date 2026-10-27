import type { SessionEvent } from "#gen/cxz/session_pb";
import { isTranscriptEvent, payload } from "./journal";
import { isDiagnosticEvent } from "./event-diagnostic";

export type WorkGroup = {
  id: string;
  anchor: SessionEvent;
  events: SessionEvent[];
  hasBoundary: boolean;
  durationMs?: number;
  services: Record<"cxz_memory" | "cxz_tasks", number>;
};

// Codex also records MCP tool permission checks as elicitations, without a
// corresponding tool row. Only settled tool checks belong in task history;
// pending decisions and genuine MCP forms/URL requests stay in the conversation.
function resolvedMcpService(event: SessionEvent) {
  if (
    event.kind !== "approval" ||
    event.text !== "mcpServer/elicitation/request"
  )
    return;
  const data = payload(event);
  if (
    data.params?.mode !== "form" ||
    data.params?._meta?.codex_approval_kind !== "mcp_tool_call" ||
    !["allowed", "denied", "canceled"].includes(data.cxz_resolution?.state)
  )
    return;
  const server: unknown = data.params?.serverName;
  return server === "cxz_memory" || server === "cxz_tasks" ? server : undefined;
}

function workService(event: SessionEvent) {
  const service = resolvedMcpService(event);
  if (service) return service;
  if (!event.toolSummary && event.kind !== "tool_call") return;
  const server = event.toolSummary?.mcpServer;
  if (server)
    return server === "cxz_memory" || server === "cxz_tasks"
      ? server
      : undefined;
  // Older Claude rows already carry the server in their native tool name.
  const name = event.toolSummary?.name || event.text;
  const match =
    /^(?:mcp__(cxz_memory|cxz_tasks)__|(cxz_memory|cxz_tasks)[._])/.exec(name);
  return (match?.[1] ?? match?.[2]) as "cxz_memory" | "cxz_tasks" | undefined;
}
// Keep conversation and actionable/important notices visible. Everything else
// belongs in the work dialog, including provider-specific and future work kinds.
const conversationKinds = new Set([
  "input",
  "assistant",
  "approval",
  "history_trimmed",
]);
export const isWorkEvent = (event: SessionEvent) =>
  isTranscriptEvent(event) &&
  (!conversationKinds.has(event.kind) || !!resolvedMcpService(event)) &&
  !isDiagnosticEvent(event) &&
  !(
    event.kind === "turn_end" && ["failed", "interrupted"].includes(event.text)
  );

// Conversation replies delimit work batches; runtime IDs keep executions isolated.
// Rows already represent executions, not their individual output/result updates.
export function workGroups(events: SessionEvent[]) {
  const active = new Map<string, WorkGroup>();
  const boundaries = new Set<string>();
  const starts = new Map<string, bigint>();
  const groups = new Map<string, WorkGroup>();
  const summaries = new Map<bigint, WorkGroup>();
  const rows: SessionEvent[] = [];
  for (const event of events) {
    if (isWorkEvent(event)) {
      let group = active.get(event.runId);
      if (!group) {
        group = {
          id: `${event.runId}/work/${event.seq}`,
          anchor: event,
          events: [],
          hasBoundary: boundaries.has(event.runId),
          services: { cxz_memory: 0, cxz_tasks: 0 },
        };
        active.set(event.runId, group);
        groups.set(group.id, group);
        summaries.set(event.seq, group);
        rows.push(event);
      }
      group.events.push(event);
      const service = workService(event);
      if (service) group.services[service]++;
    } else rows.push(event);
    if (["input", "assistant", "turn_end"].includes(event.kind)) {
      const group = active.get(event.runId);
      const start = starts.get(event.runId);
      if (group && start !== undefined && start > 0n && event.timeMs >= start)
        group.durationMs = Number(event.timeMs - start);
      active.delete(event.runId);
      if (event.kind !== "turn_end") {
        boundaries.add(event.runId);
        starts.set(event.runId, event.timeMs);
      } else starts.delete(event.runId);
    }
  }
  return { events: rows, groups, summaries };
}

// Paging can replace an orphan result's row with its earlier call. Keep the
// dialog attached to the execution when its display anchor or turn key changes.
export function selectedWorkGroup(
  groups: Map<string, WorkGroup>,
  selected?: SessionEvent,
) {
  if (!selected) return;
  return [...groups.values()].find((group) =>
    group.events.some(
      (event) =>
        event.seq === selected.seq ||
        (selected.requestId &&
          event.requestId === selected.requestId &&
          event.runId === selected.runId),
    ),
  );
}
