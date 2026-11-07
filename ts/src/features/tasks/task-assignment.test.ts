import { create } from "@bufbuild/protobuf";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SessionSchema } from "#gen/cxz/session_pb";
import { SessionReceiptSchema } from "#gen/cxz/session_svc_pb";
import { TaskStatus } from "#gen/cxz/task_pb";
import { taskFixture } from "#src/storybook/task-fixture.ts";
import { taskLookup } from "./task-model";
import {
  taskAssignmentAction,
  randomAssignee,
  taskReviewRequest,
  sendTaskReview,
} from "./task-assignment";

const cleanups: (() => void)[] = [];
afterEach(() => {
  cleanups.splice(0).forEach((close) => close());
  vi.restoreAllMocks();
});
function fixture() {
  const value = taskFixture();
  cleanups.push(() => value.c.dispose());
  return value;
}
async function task(f: ReturnType<typeof fixture>, alias = "task-7") {
  return f.c.tasks.get({
    ref: taskLookup(f.projects[0], alias),
    select: { all: true },
  });
}

describe("Task assignment actions", () => {
  it("randomly picks only active, current-run sessions in the same project", async () => {
    const f = fixture();
    const todo = await task(f, "task-1");
    const idle = create(SessionSchema, {
      runtimeId: "pine",
      project: f.projects[0],
      status: { state: "idle", runId: "pine-run" },
    });
    const waiting = create(SessionSchema, {
      runtimeId: "ash",
      project: f.projects[0],
      status: { state: "waiting_input", runId: "ash-run" },
    });
    const others = [
      create(SessionSchema, {
        project: f.projects[1],
        status: { state: "idle", runId: "other-run" },
      }),
      create(SessionSchema, {
        project: f.projects[0],
        status: { state: "idle" },
      }),
      create(SessionSchema, {
        project: f.projects[0],
        status: { state: "starting", runId: "starting-run" },
      }),
    ];
    const action = taskAssignmentAction(todo, f.projects[0], [
      ...f.sessions,
      idle,
      waiting,
      ...others,
    ]);
    expect(action.kind).toBe("random");
    if (action.kind !== "random") throw new Error("Expected random assignment");
    expect(action.sessions.map((s) => s.runtimeId)).toEqual([
      "oak",
      "pine",
      "ash",
    ]);
    expect(randomAssignee(action.sessions, () => 0)).toBe(f.sessions[0]);
    expect(randomAssignee(action.sessions, () => 0.5)).toBe(idle);
    expect(randomAssignee(action.sessions, () => 0.99)).toBe(waiting);
    expect(randomAssignee([])).toBeUndefined();
    expect(taskAssignmentAction(todo, f.projects[0], []).kind).toBe("random");
  });

  it("offers review only for previous-run active owners and locks terminal tasks", async () => {
    const f = fixture();
    for (const alias of ["task-2", "task-3", "task-5", "task-6", "task-8"])
      expect(
        taskAssignmentAction(await task(f, alias), f.projects[0], f.sessions),
      ).toEqual({ kind: "none" });
    const previous = await task(f);
    expect(taskAssignmentAction(previous, f.projects[0], f.sessions)).toEqual({
      kind: "review",
      session: f.sessions[0],
    });
    previous.status = TaskStatus.WAITING;
    expect(taskAssignmentAction(previous, f.projects[0], f.sessions).kind).toBe(
      "review",
    );
    expect(taskAssignmentAction(previous, f.projects[0], []).kind).toBe("none");
    f.sessions[0].status!.state = "stopped";
    expect(taskAssignmentAction(previous, f.projects[0], f.sessions).kind).toBe(
      "none",
    );
  });

  it("captures UUID, saved task text and run once, sends idempotently without changing ownership or drafts", async () => {
    const f = fixture();
    const previous = await task(f);
    const request = taskReviewRequest(previous, f.sessions[0]);
    expect(request.text).toContain("Restore interrupted work");
    expect(request.text).toContain("alias: task-7");
    expect(request.text).toContain("00000000-0000-0000-0000-000000000010");
    expect(request.text).toContain("task_get");
    expect(request.runId).toBe("oak-run");
    f.c.drafts.set("oak", "Keep my current composer draft");
    const options = { timeoutMs: 1000 };
    expect(
      (await sendTaskReview(f.c, f.projects[0], previous, request, options))
        .status,
    ).toBe("queued");
    await sendTaskReview(f.c, f.projects[0], previous, request, options);
    expect(f.reviews.size).toBe(1);
    expect(f.reviews.get(request.clientId)?.text).toBe(request.text);
    expect((await task(f)).assigneeRun).toBe("old-oak-run");
    expect(f.c.drafts.get("oak")).toBe("Keep my current composer draft");
  });

  it("does not send after a task version or owner changes while confirmation is open", async () => {
    const f = fixture();
    const previous = await task(f);
    const request = taskReviewRequest(previous, f.sessions[0]);
    await f.c.tasks.patch({
      ref: taskLookup(f.projects[0], "task-7"),
      dateUpdated: previous.dateUpdated,
      name: "Changed task",
    });
    await expect(
      sendTaskReview(f.c, f.projects[0], previous, request, {}),
    ).rejects.toThrow("This task changed");
    expect(f.reviews.size).toBe(0);
  });

  it("does not retarget a confirmation to a new session run", async () => {
    const f = fixture();
    const previous = await task(f);
    const request = taskReviewRequest(previous, f.sessions[0]);
    f.sessions[0].status!.runId = "next-oak-run";
    expect(request.runId).toBe("oak-run");
    await expect(
      sendTaskReview(f.c, f.projects[0], previous, request, {}),
    ).rejects.toThrow("Session run changed");
    expect(f.reviews.size).toBe(0);
  });

  it("surfaces a rejected session receipt as a failure", async () => {
    const f = fixture();
    const previous = await task(f);
    vi.spyOn(f.c.sessions, "send").mockResolvedValue(
      create(SessionReceiptSchema, { status: "rejected" }),
    );
    await expect(
      sendTaskReview(
        f.c,
        f.projects[0],
        previous,
        taskReviewRequest(previous, f.sessions[0]),
        {},
      ),
    ).rejects.toThrow("rejected the review request");
  });
});
