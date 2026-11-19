import { describe, expect, it } from "vitest";
import { create } from "@bufbuild/protobuf";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import { subagentDurationMs, subagentGroups } from "./subagent-groups";
import { workGroups } from "./work-groups";

let seq = 0n;
const event = (
  kind: string,
  agent?: { id: string; type?: string },
  extra: Record<string, unknown> = {},
) =>
  create(SessionEventSchema, {
    seq: ++seq,
    runId: "run",
    kind,
    timeMs: seq * 1000n,
    agent: agent && { id: agent.id, type: agent.type ?? "general-purpose" },
    ...extra,
  });

describe("grouping a subagent's work", () => {
  it("leaves the session's own events where they are", () => {
    const own = [event("input"), event("assistant"), event("tool_call")];
    const grouped = subagentGroups(own);
    expect(grouped.events).toEqual(own);
    expect(grouped.groups.size).toBe(0);
    expect(grouped.summaries.size).toBe(0);
  });

  it("collects a subagent under one row, anchored at its first event", () => {
    const first = event("assistant", { id: "toolu_a" });
    const rows = [
      event("input"),
      first,
      event("tool_call", { id: "toolu_a" }),
      event("tool_result", { id: "toolu_a" }),
      event("assistant", { id: "toolu_a" }),
      event("assistant"),
    ];
    const grouped = subagentGroups(rows);
    expect(grouped.events.map((e) => e.seq)).toEqual([
      rows[0]!.seq,
      first.seq,
      rows[5]!.seq,
    ]);
    const group = grouped.summaries.get(first.seq)!;
    expect(group.id).toBe("toolu_a");
    expect(group.type).toBe("general-purpose");
    expect(group.events.length).toBe(4);
    expect(group.messages).toBe(2);
    expect(group.work).toBe(2);
  });

  // Several subagents share the session's run, so the run is not what tells
  // them apart; the call that started each one is.
  it("keeps concurrent subagents apart even when their events interleave", () => {
    const rows = [
      event("assistant", { id: "toolu_a" }),
      event("assistant", { id: "toolu_b" }),
      event("tool_call", { id: "toolu_a" }),
      event("assistant", { id: "toolu_c" }),
      event("tool_call", { id: "toolu_b" }),
      event("tool_call", { id: "toolu_c" }),
    ];
    const grouped = subagentGroups(rows);
    expect(grouped.groups.size).toBe(3);
    expect(grouped.events.length).toBe(3);
    for (const id of ["toolu_a", "toolu_b", "toolu_c"])
      expect(grouped.groups.get(id)!.events.length).toBe(2);
  });

  // The session's work groups are keyed by run, which a subagent shares. Its
  // tool calls used to land in the session's own group.
  it("keeps a subagent's tool activity out of the session's work groups", () => {
    const mine = event("tool_call");
    const grouped = subagentGroups([
      mine,
      event("tool_call", { id: "toolu_a" }),
      event("tool_result", { id: "toolu_a" }),
    ]);
    const work = workGroups(grouped.events);
    const group = work.summaries.get(mine.seq);
    expect(group?.events.length).toBe(1);
    for (const g of work.groups.values())
      for (const e of g.events) expect(e.agent?.id ?? "").toBe("");
  });

  it("times a group by the events it holds", () => {
    const grouped = subagentGroups([
      event("assistant", { id: "toolu_a" }),
      event("assistant", { id: "toolu_a" }),
    ]);
    const group = grouped.groups.get("toolu_a")!;
    expect(subagentDurationMs(group)).toBe(1000);
  });

  it("answers no duration when it cannot know one", () => {
    const grouped = subagentGroups([
      event("assistant", { id: "toolu_a" }, { timeMs: 0n }),
    ]);
    expect(subagentDurationMs(grouped.groups.get("toolu_a")!)).toBeUndefined();
  });

  // Records made before the field existed carry nothing. Treating them as the
  // session's own would be a guess; leaving them in place is what they are.
  it("leaves records with no agent unattributed", () => {
    const rows = [event("assistant"), event("tool_call")];
    expect(subagentGroups(rows).groups.size).toBe(0);
  });
});
