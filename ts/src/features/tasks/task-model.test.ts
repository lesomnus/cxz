import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { TaskSchema, TaskStatus } from "#gen/cxz/task_pb";
import { SessionSchema } from "#gen/cxz/session_pb";
import { boardTasks, ownerIssue } from "./task-model";

describe("task board projection", () => {
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
  it("filters a session run and keeps recent completions ahead of older ones", () => {
    const tasks = Array.from({ length: 20 }, (_, i) =>
      create(TaskSchema, {
        alias: `task-${i}`,
        status: TaskStatus.DONE,
        assignee: "oak",
        assigneeRun: "run",
        dateUpdated: { seconds: BigInt(i) },
      }),
    );
    tasks.push(
      create(TaskSchema, {
        status: TaskStatus.IN_PROGRESS,
        assignee: "oak",
        assigneeRun: "old-run",
      }),
    );
    const board = boardTasks(tasks, "oak", "run");
    expect(
      board.find((column) => column.value === TaskStatus.IN_PROGRESS)?.total,
    ).toBe(0);
    const done = board.find((column) => column.value === TaskStatus.DONE)!;
    expect(done.items[0].alias).toBe("task-19");
    expect(done.items.length).toBeLessThan(done.total);
    expect(
      boardTasks(tasks, "oak", "run", true).find(
        (column) => column.value === TaskStatus.DONE,
      )?.items,
    ).toHaveLength(20);
  });
});
