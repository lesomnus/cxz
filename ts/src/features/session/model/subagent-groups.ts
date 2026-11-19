import type { SessionEvent } from "#gen/cxz/session_pb";

// Counted rather than classified: `isWorkEvent` answers what belongs in the
// session's work history, which a subagent's events deliberately do not.
const toolKinds = new Set(["tool_call", "tool_result", "tool_output"]);

export type SubagentGroup = {
  /** The id of the `Agent` call that started this subagent. */
  id: string;
  type: string;
  anchor: SessionEvent;
  events: SessionEvent[];
  /** How many of its events are conversation rather than tool activity. */
  messages: number;
  work: number;
  fromMs?: bigint;
  toMs?: bigint;
};

// A subagent's replies and tool calls arrive on the session's own stream, so
// without grouping they read as the session talking to itself -- and with
// several running at once, as several sessions talking over each other. They
// are collected under the call that started each one, which also keeps a
// subagent's tool activity out of the session's own work groups: those are
// keyed by run, and a subagent shares the session's run.
//
// Events recorded before cxz#368 carry no agent. They are left where they are
// rather than guessed at: unattributed is not the session's own.
export function subagentGroups(events: SessionEvent[]) {
  const groups = new Map<string, SubagentGroup>();
  const summaries = new Map<bigint, SubagentGroup>();
  const rows: SessionEvent[] = [];
  for (const event of events) {
    const id = event.agent?.id;
    if (!id) {
      rows.push(event);
      continue;
    }
    let group = groups.get(id);
    if (!group) {
      group = {
        id,
        type: event.agent?.type ?? "",
        anchor: event,
        events: [],
        messages: 0,
        work: 0,
        fromMs: event.timeMs || undefined,
      };
      groups.set(id, group);
      summaries.set(event.seq, group);
      rows.push(event);
    }
    group.events.push(event);
    if (toolKinds.has(event.kind)) group.work++;
    else group.messages++;
    if (event.timeMs > 0n) group.toMs = event.timeMs;
    if (!group.type && event.agent?.type) group.type = event.agent.type;
  }
  return { events: rows, groups, summaries };
}

export function subagentDurationMs(group: SubagentGroup) {
  if (group.fromMs === undefined || group.toMs === undefined) return;
  if (group.toMs < group.fromMs) return;
  return Number(group.toMs - group.fromMs);
}
