import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import {
  SessionEventSchema,
  ResponseMetadataSchema,
} from "#gen/cxz/session_pb";
import { transcriptEvents } from "./tool-activity";
import { workGroups, selectedWorkGroup } from "./work-groups";

const event = (seq: number, kind: string, requestId = "", runId = "run") =>
  create(SessionEventSchema, { seq: BigInt(seq), kind, requestId, runId });

describe("work between conversation responses", () => {
  it("counts executions once and keeps commentary, answers, questions and errors in the conversation", () => {
    const events = [
      event(1, "input"),
      event(2, "assistant"),
      event(3, "tool_call", "bash"),
      event(4, "tool_output", "bash"),
      event(5, "tool_result", "bash"),
      event(6, "assistant"),
      event(7, "tool_call", "edit"),
      event(8, "approval"),
      event(9, "diagnostic"),
      event(10, "assistant"),
    ];
    const projected = workGroups(transcriptEvents(events).events);
    expect(projected.events.map((e) => e.seq)).toEqual([
      1n,
      2n,
      3n,
      6n,
      7n,
      8n,
      9n,
      10n,
    ]);
    expect(
      [...projected.groups.values()].map((group) =>
        group.events.map((e) => e.seq),
      ),
    ).toEqual([[3n], [7n]]);
    expect(events[2].kind).toBe("tool_call");
  });
  it("separates repeated turns within the same run and interleaved different runs", () => {
    const events = [
      event(1, "input"),
      event(2, "tool_call", "a"),
      event(3, "input", "", "other"),
      event(4, "tool_call", "b", "other"),
      event(5, "tool_result", "c"),
      event(6, "input"),
      event(7, "tool_call", "d"),
    ];
    const projected = workGroups(events);
    expect(
      [...projected.groups.values()].map((group) =>
        group.events.map((e) => e.seq),
      ),
    ).toEqual([[2n, 5n], [4n], [7n]]);
  });
  it("uses explicit turn completion when an autonomous turn has no new input", () => {
    const final = event(3, "assistant");
    final.response = create(ResponseMetadataSchema, {
      phase: "final_answer",
      completionJson: new TextEncoder().encode("{}"),
    });
    const projected = workGroups([
      event(1, "tool_call", "a"),
      event(2, "assistant"),
      final,
      event(4, "tool_call", "b"),
      event(5, "turn_end"),
      event(6, "tool_call", "c"),
    ]);
    expect(
      [...projected.groups.values()].map((group) =>
        group.events.map((e) => e.seq),
      ),
    ).toEqual([[1n], [4n], [6n]]);
  });
  it("keeps dialog selection when history prepends the missing call and input", () => {
    const result = event(5, "tool_result", "execution");
    const before = workGroups(transcriptEvents([result]).events);
    const after = workGroups(
      transcriptEvents([
        event(1, "input"),
        event(2, "tool_call", "execution"),
        result,
      ]).events,
    );
    expect(selectedWorkGroup(before.groups, result)?.anchor.seq).toBe(5n);
    expect(selectedWorkGroup(after.groups, result)?.anchor.seq).toBe(2n);
    expect(
      selectedWorkGroup(after.groups, event(99, "tool_call", "another")),
    ).toBeUndefined();
  });
  it("reduces a large turn to a bounded conversation row set without losing task records", () => {
    const tasks = Array.from({ length: 4000 }, (_, i) =>
      event(i + 2, "tool_call", `task-${i}`),
    );
    const projected = workGroups([
      event(1, "input"),
      ...tasks,
      event(4002, "assistant"),
    ]);
    expect(projected.events).toHaveLength(3);
    expect([...projected.groups.values()][0].events).toHaveLength(4000);
  });
});
