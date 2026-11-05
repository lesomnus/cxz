import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { TaskSchema, TaskStatus } from "#gen/cxz/task_pb";
import { SessionSchema } from "#gen/cxz/session_pb";
import { tableTasks, ownerIssue } from "./task-model";

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
