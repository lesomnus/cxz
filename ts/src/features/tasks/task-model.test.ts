import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { TaskSchema, TaskStatus } from "#gen/cxz/task_pb";
import { SessionSchema } from "#gen/cxz/session_pb";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import {
  tableTasks,
  taskTablePage,
  taskHistoryPageSize,
  ownerIssue,
} from "./task-model";

describe("task table projection", () => {
  it("distinguishes stopped, previous-run and removed owners without changing the task", () => {
    const task = create(TaskSchema, {
      status: TaskStatus.IN_PROGRESS,
      assignee: "oak",
      assigneeRun: "run-1",
    });
    const session = create(SessionSchema, {
      runtimeId: "oak",
      status: { runId: "run-1", state: "waiting_input" },
    });
    expect(ownerIssue(task, [session])).toBe("");
    session.status!.state = "stopped";
    expect(ownerIssue(task, [session])).toBe("Session stopped");
    session.status!.runId = "run-2";
    expect(ownerIssue(task, [session])).toBe("Previous run");
    expect(ownerIssue(task, [], true)).toBe("");
    expect(ownerIssue(task, [])).toBe("Session unavailable");
    expect(task.status).toBe(TaskStatus.IN_PROGRESS);
    task.status = TaskStatus.DONE;
    expect(ownerIssue(task, [])).toBe("");
  });
  it("orders by creation time rather than priority or updates, putting Done last", () => {
    const row = (
      alias: string,
      status: TaskStatus,
      seconds: bigint,
      nanos = 0,
    ) => create(TaskSchema, { alias, status, dateCreated: { seconds, nanos } });
    const tasks = [
      row("old-todo", TaskStatus.TODO, 10n),
      row("new-done", TaskStatus.DONE, 100n),
      row("old-done", TaskStatus.DONE, 5n),
      row("working", TaskStatus.IN_PROGRESS, 20n, 1),
      row("canceled", TaskStatus.CANCELED, 30n),
      row("waiting", TaskStatus.WAITING, 20n, 2),
    ];
    tasks[0].priority = 0;
    tasks[0].dateUpdated = {
      $typeName: "google.protobuf.Timestamp",
      seconds: 200n,
      nanos: 0,
    };
    expect(tableTasks(tasks).map((task) => task.alias)).toEqual([
      "canceled",
      "waiting",
      "working",
      "old-todo",
      "new-done",
      "old-done",
    ]);
    expect(tasks[0].alias).toBe("old-todo");
  });
  it("keeps session/run filters and includes every completed task", () => {
    const tasks = Array.from({ length: 20 }, (_, i) =>
      create(TaskSchema, {
        alias: `task-${i}`,
        status: TaskStatus.DONE,
        assignee: "oak",
        assigneeRun: "run",
        dateCreated: { seconds: BigInt(i) },
      }),
    );
    tasks.push(
      create(TaskSchema, {
        status: TaskStatus.IN_PROGRESS,
        assignee: "oak",
        assigneeRun: "old-run",
      }),
      create(TaskSchema, {
        status: TaskStatus.IN_PROGRESS,
        assignee: "elm",
        assigneeRun: "run",
      }),
    );
    const result = tableTasks(tasks, "oak", "run");
    expect(result).toHaveLength(20);
    expect(result[0].alias).toBe("task-19");
    expect(result.every((task) => task.status === TaskStatus.DONE)).toBe(true);
  });
});

describe("completed Task history", () => {
  const now = new Date(2026, 10, 2, 15, 30);
  const yesterday = new Date(2026, 10, 1);
  const row = (alias: string, updated?: Date, status = TaskStatus.DONE) =>
    create(TaskSchema, {
      alias,
      status,
      dateCreated: timestampFromDate(new Date(2026, 0, 1)),
      dateUpdated: updated && timestampFromDate(updated),
    });

  it("keeps today and all of yesterday using local midnight, with older unfinished work always visible", () => {
    const tasks = [
      row("old-active", new Date(2026, 0, 1), TaskStatus.IN_PROGRESS),
      row("old-canceled", new Date(2026, 0, 1), TaskStatus.CANCELED),
      row("today", now),
      row("yesterday-start", yesterday),
      row("before-yesterday", new Date(yesterday.getTime() - 1)),
      row("missing-update"),
    ];
    tasks.push(
      create(TaskSchema, {
        alias: "created-yesterday",
        status: TaskStatus.DONE,
        dateCreated: timestampFromDate(yesterday),
      }),
    );
    const page = taskTablePage(tasks, now.getTime());
    expect(page.rows.map((task) => task.alias)).toEqual([
      "old-active",
      "old-canceled",
      "today",
      "yesterday-start",
      "created-yesterday",
    ]);
    expect(page.hidden).toBe(2);
    expect(tasks).toHaveLength(7);
  });

  it("reveals old completions in batches of 50 while retaining the sorted order", () => {
    const tasks = Array.from({ length: 125 }, (_, i) => row(`old-${i}`));
    tasks.splice(25, 0, row("recent", now));
    tasks.unshift(row("active", undefined, TaskStatus.TODO));
    expect(
      taskTablePage(tasks, now.getTime()).rows.map((t) => t.alias),
    ).toEqual(["active", "recent"]);
    for (const [limit, visible, hidden] of [
      [50, 52, 75],
      [100, 102, 25],
      [150, 127, 0],
    ]) {
      const page = taskTablePage(tasks, now.getTime(), limit);
      expect(page.rows).toHaveLength(visible);
      expect(page.hidden).toBe(hidden);
      expect(page.rows).toEqual(
        tasks.filter(
          (t) =>
            t.alias === "active" ||
            t.alias === "recent" ||
            Number(t.alias.slice(4)) < limit,
        ),
      );
    }
    expect(taskHistoryPageSize).toBe(50);
  });

  it("counts only the selected session and run, and hides undated completions", () => {
    const tasks = [row("mine"), row("other-run"), row("other-owner")];
    Object.assign(tasks[0], { assignee: "oak", assigneeRun: "run" });
    Object.assign(tasks[1], { assignee: "oak", assigneeRun: "old-run" });
    Object.assign(tasks[2], { assignee: "elm", assigneeRun: "run" });
    const filtered = tableTasks(tasks, "oak", "run");
    expect(taskTablePage(filtered, now.getTime())).toMatchObject({
      rows: [],
      hidden: 1,
    });
    expect(
      taskTablePage(filtered, now.getTime(), taskHistoryPageSize).rows,
    ).toEqual(filtered);
    expect(
      taskTablePage(
        [create(TaskSchema, { status: TaskStatus.DONE })],
        now.getTime(),
      ).hidden,
    ).toBe(1);
  });
});
