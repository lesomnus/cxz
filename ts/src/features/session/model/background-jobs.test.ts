import { create } from "@bufbuild/protobuf";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import { describe, expect, it } from "vitest";
import { BackgroundJobs } from "./background-jobs";
const bytes = (value: unknown) =>
  new TextEncoder().encode(JSON.stringify(value));
const event = (seq: number, data: unknown, run = "run") =>
  create(SessionEventSchema, {
    seq: BigInt(seq),
    runId: run,
    kind: "background",
    payload: bytes({ type: "system", ...(data as object) }),
  });
const seed = (jobs: BackgroundJobs, tasks: unknown[] = [], seq = 1) =>
  jobs.seed(
    bytes({
      run: {
        Snapshot: true,
        Tasks: Object.fromEntries(
          tasks.map((task: any) => [task.task_id, task]),
        ),
      },
    }),
    BigInt(seq),
  );

describe("provider-reported background jobs", () => {
  it("distinguishes missing telemetry from a confirmed empty snapshot", () => {
    const jobs = new BackgroundJobs("run");
    jobs.seed(bytes({}), 1n);
    expect(jobs.snapshot().count).toBeUndefined();
    seed(jobs);
    expect(jobs.snapshot().count).toBe(0);
  });
  it("counts current-run active flags, not old runs or working status alone", () => {
    const jobs = new BackgroundJobs("run");
    jobs.seed(
      bytes({
        old: { Tasks: { old: { task_id: "old", active: true } } },
        run: {
          Tasks: {
            a: { task_id: "a", active: true },
            b: { task_id: "b", status: "working", active: false },
            malformed: null,
          },
        },
      }),
      1n,
    );
    expect(jobs.snapshot().count).toBe(1);
    jobs.receive(
      event(
        2,
        { subtype: "task_started", task_id: "old", is_backgrounded: true },
        "old",
      ),
    );
    expect(jobs.snapshot().count).toBe(1);
  });
  it("ignores foreground starts, launch results and unknown completion notices", () => {
    const jobs = new BackgroundJobs("run");
    seed(jobs);
    jobs.receive(
      event(2, {
        subtype: "task_started",
        task_id: "foreground",
        is_backgrounded: false,
      }),
    );
    jobs.receive(
      event(3, {
        subtype: "task_notification",
        task_id: "unknown",
        status: "working",
      }),
    );
    jobs.receive(
      create(SessionEventSchema, {
        seq: 4n,
        runId: "run",
        kind: "tool_result",
        text: "Started a background job",
      }),
    );
    expect(jobs.snapshot().count).toBe(0);
  });
  it("updates on starts, patches and authoritative snapshots even after a foreground turn ends", () => {
    const jobs = new BackgroundJobs("run");
    seed(jobs);
    jobs.receive(
      event(2, {
        subtype: "task_started",
        task_id: "a",
        is_backgrounded: true,
      }),
    );
    jobs.receive(
      event(3, {
        subtype: "task_started",
        task_id: "b",
        is_backgrounded: true,
      }),
    );
    expect(jobs.snapshot().count).toBe(2);
    jobs.receive(
      create(SessionEventSchema, {
        seq: 4n,
        runId: "run",
        kind: "turn_end",
        text: "completed",
      }),
    );
    expect(jobs.snapshot().count).toBe(2);
    jobs.receive(
      event(5, {
        subtype: "task_updated",
        task_id: "a",
        patch: { status: "completed" },
      }),
    );
    expect(jobs.snapshot().count).toBe(1);
    jobs.receive(event(6, { subtype: "background_tasks_changed", tasks: [] }));
    expect(jobs.snapshot().count).toBe(0);
  });
  it("replays live updates newer than a delayed snapshot fence and deduplicates reconnects", () => {
    const jobs = new BackgroundJobs("run");
    jobs.receive(
      event(5, {
        subtype: "task_started",
        task_id: "a",
        is_backgrounded: true,
      }),
    );
    jobs.receive(
      event(7, {
        subtype: "task_notification",
        task_id: "a",
        status: "completed",
      }),
    );
    seed(jobs, [{ task_id: "a", active: true, status: "working" }], 6);
    expect(jobs.snapshot().count).toBe(0);
    jobs.receive(
      event(5, {
        subtype: "task_started",
        task_id: "a",
        is_backgrounded: true,
      }),
    );
    expect(jobs.snapshot().count).toBe(0);
    jobs.beginSnapshot();
    seed(jobs, [{ task_id: "a", active: true }], 3);
    expect(jobs.snapshot().count).toBe(0);
  });
  for (const status of [
    "completed",
    "failed",
    "stopped",
    "canceled",
    "cancelled",
  ])
    it(`stops counting ${status} tasks`, () => {
      const jobs = new BackgroundJobs("run");
      seed(jobs, [{ task_id: "a", active: true }]);
      jobs.receive(
        event(2, { subtype: "task_notification", task_id: "a", status }),
      );
      expect(jobs.snapshot().count).toBe(0);
    });
});
