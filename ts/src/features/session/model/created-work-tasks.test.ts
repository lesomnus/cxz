import { create } from "@bufbuild/protobuf";
import { TimestampSchema } from "@bufbuild/protobuf/wkt";
import { describe, expect, it } from "vitest";
import { TaskSchema } from "#gen/cxz/task_pb";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import {
  createdWorkTasks,
  createdTasksByWorkGroup,
} from "./created-work-tasks";
import { workGroups } from "./work-groups";

const event = (seq: number, kind: string, ms: number, runId = "run") =>
  create(SessionEventSchema, {
    seq: BigInt(seq),
    kind,
    timeMs: BigInt(ms),
    runId,
  });
const task = (alias: string, ms: number, session = "oak", run = "run") =>
  create(TaskSchema, {
    alias,
    sourceSession: session,
    sourceRun: run,
    dateCreated: {
      seconds: BigInt(Math.floor(ms / 1000)),
      nanos: (ms % 1000) * 1_000_000,
    },
  });
const batch = () =>
  [
    ...workGroups([
      event(1, "assistant", 1000),
      event(2, "tool_call", 2000),
      event(3, "assistant", 3000),
    ]).groups.values(),
  ][0];

describe("Tasks created in a reply-delimited work range", () => {
  it("indexes multiple reply ranges once and keeps tasks out of other sessions and runs", () => {
    const groups = workGroups([
      event(1, "input", 1000),
      event(2, "tool_call", 1500),
      event(3, "assistant", 2000),
      event(4, "tool_call", 2500),
      event(5, "assistant", 3000),
      event(6, "input", 1000, "other"),
      event(7, "tool_call", 2000, "other"),
      event(8, "assistant", 3000, "other"),
    ]).groups;
    const tasks = [
      task("second", 2500),
      task("first", 1500),
      task("boundary", 2000),
      task("other", 2000, "oak", "other"),
      task("wrong-session", 2000, "elm"),
      task("before", 999),
      task("after", 3000),
    ];
    const indexed = createdTasksByWorkGroup(
      tasks,
      "oak",
      [...groups.values()].reverse(),
    );
    expect([...indexed.get("run/work/2")!].map((t) => t.alias)).toEqual([
      "first",
    ]);
    expect(indexed.get("run/work/4")!.map((t) => t.alias)).toEqual([
      "boundary",
      "second",
    ]);
    expect(indexed.get("other/work/7")!.map((t) => t.alias)).toEqual(["other"]);
    expect([...indexed.values()].flat()).toHaveLength(4);
  });
  it("filters creation provenance and uses half-open nanosecond-accurate bounds", () => {
    const group = batch();
    expect(group.fromMs).toBe(1000n);
    expect(group.toMs).toBe(3000n);
    const within = task("within", 2000);
    const lower = task("lower", 1000);
    const justBeforeEnd = task("before-end", 2999);
    justBeforeEnd.dateCreated!.nanos += 999999;
    const edited = task("edited-later", 100);
    edited.dateUpdated = create(TimestampSchema, { seconds: 2n, nanos: 0 });
    expect(
      createdWorkTasks(
        [
          within,
          lower,
          justBeforeEnd,
          task("end", 3000),
          task("old", 999),
          edited,
          task("other-session", 2000, "elm"),
          task("other-run", 2000, "oak", "old-run"),
          create(TaskSchema, {
            alias: "missing-time",
            sourceSession: "oak",
            sourceRun: "run",
          }),
          task("manual", 2000, "", ""),
        ],
        "oak",
        group,
      ).map((t) => t.alias),
    ).toEqual(["lower", "within", "before-end"]);
  });

  it("keeps concurrent runs isolated and allows live ranges to receive new Tasks", () => {
    const groups = [
      ...workGroups([
        event(1, "input", 1000),
        event(2, "input", 1500, "other"),
        event(3, "tool_call", 2000),
        event(4, "tool_call", 2500, "other"),
        event(5, "assistant", 3000, "other"),
      ]).groups.values(),
    ];
    expect(groups[0].toMs).toBeUndefined();
    expect(groups[1].fromMs).toBe(1500n);
    expect(groups[1].toMs).toBe(3000n);
    const tasks = [task("live", 4000), task("other", 2700, "oak", "other")];
    expect(
      createdWorkTasks(tasks, "oak", groups[0]).map((t) => t.alias),
    ).toEqual(["live"]);
  });

  it("limits partial history to its first loaded event and requires known timestamps", () => {
    const group = [
      ...workGroups([event(1, "tool_call", 2000)]).groups.values(),
    ][0];
    expect(group.hasBoundary).toBe(false);
    expect(
      createdWorkTasks(
        [task("old", 1000), task("loaded", 2000)],
        "oak",
        group,
      ).map((t) => t.alias),
    ).toEqual(["loaded"]);
    expect(
      createdWorkTasks([task("unknown", 2000)], "oak", {
        ...group,
        fromMs: undefined,
      }),
    ).toEqual([]);
    expect(createdWorkTasks([task("unknown", 2000)], "", group)).toEqual([]);
  });
});
