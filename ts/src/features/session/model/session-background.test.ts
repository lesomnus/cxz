import { create } from "@bufbuild/protobuf";
import { afterEach, expect, it, vi } from "vitest";
import { Id } from "@lesomnus/payday/pdid";
import { SessionSchema } from "#gen/cxz/session_pb";
import { ContainerState, ProjectSchema } from "#gen/cxz/project_pb";
import { SessionBackground } from "./session-background";

const project = create(ProjectSchema, {
  id: new Uint8Array(16).fill(1),
  status: { containerState: ContainerState.RUNNING },
});
const projects = [project];

const session = (
  state = "idle",
  seq = 1n,
  run = "run",
  runtimeId = "session",
) =>
  create(SessionSchema, {
    id: Uint8Array.from([
      0,
      0,
      0,
      0,
      0,
      0,
      0x80,
      0,
      0x80,
      8,
      0,
      0,
      0,
      0,
      0,
      runtimeId === "session" ? 1 : Number(runtimeId) + 2,
    ]),
    runtimeId,
    project: { id: project.id },
    status: { state, lastSeq: seq, runId: run },
  });
const reply = (active: boolean, run = "run", unavailable = false) => ({
  data: new TextEncoder().encode(
    JSON.stringify({
      [run]: {
        Unavailable: unavailable,
        Tasks: { job: { task_id: "job", active } },
      },
    }),
  ),
  lastSeq: 1n,
});
afterEach(() => vi.useRealTimers());

it("only queries confirmed running projects and clears activity when their containers stop or disappear", async () => {
  const background = vi.fn().mockResolvedValue(reply(true));
  const tracker = new SessionBackground({ background });
  for (const state of [
    ContainerState.ABSENT,
    ContainerState.CREATED,
    ContainerState.STOPPED,
    ContainerState.UNSPECIFIED,
  ]) {
    tracker.observe(
      [session()],
      [
        create(ProjectSchema, {
          id: project.id,
          status: { containerState: state },
        }),
      ],
    );
  }
  tracker.observe([session()], []);
  expect(background).not.toHaveBeenCalled();
  tracker.observe([session()], projects);
  await vi.waitFor(() => expect(tracker.snapshot().has("session")).toBe(true));
  tracker.observe(
    [session()],
    [
      create(ProjectSchema, {
        id: project.id,
        status: { containerState: ContainerState.ABSENT },
      }),
    ],
  );
  expect(tracker.snapshot().size).toBe(0);
  tracker.observe([session()], projects);
  await vi.waitFor(() => expect(background).toHaveBeenCalledTimes(2));
  await vi.waitFor(() => expect(tracker.snapshot().has("session")).toBe(true));
  tracker.dispose();
});

it("aborts snapshots for removed containers and ignores late replies", async () => {
  let finish!: (value: ReturnType<typeof reply>) => void;
  let signal!: AbortSignal;
  const background = vi.fn((_request, options) => {
    signal = options.signal;
    return new Promise<ReturnType<typeof reply>>((resolve) => {
      finish = resolve;
    });
  });
  const tracker = new SessionBackground({ background: background as any });
  tracker.observe([session()], projects);
  tracker.observe([session()], []);
  expect(signal.aborted).toBe(true);
  finish(reply(true));
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(tracker.snapshot().size).toBe(0);
  tracker.dispose();
});

it("addresses Background by the public UUID even without a legacy runtime ID", async () => {
  const background = vi.fn().mockResolvedValue(reply(true));
  const tracker = new SessionBackground({ background });
  const record = session();
  record.runtimeId = "";
  tracker.observe([record], projects);
  await vi.waitFor(() => expect(tracker.snapshot().size).toBe(1));
  expect(background).toHaveBeenCalledWith(
    { ref: { key: { case: "id", value: record.id } } },
    expect.anything(),
  );
  expect(tracker.snapshot().has(new Id(record.id).toString())).toBe(true);
  tracker.dispose();
});

it("finds idle background work, deduplicates snapshots, and clears completed or stopped work", async () => {
  const background = vi.fn().mockResolvedValue(reply(true));
  const tracker = new SessionBackground({ background });
  tracker.observe([session()], projects);
  await vi.waitFor(() => expect(tracker.snapshot().has("session")).toBe(true));
  expect(background).toHaveBeenCalledWith(
    { ref: { key: { case: "id", value: session().id } } },
    expect.anything(),
  );
  tracker.observe([session()], projects);
  expect(background).toHaveBeenCalledTimes(1);
  background.mockResolvedValue(reply(false));
  tracker.observe([session("idle", 2n)], projects);
  await vi.waitFor(() => expect(tracker.snapshot().size).toBe(0));
  background.mockResolvedValue(reply(true));
  tracker.observe([session("idle", 3n)], projects);
  await vi.waitFor(() => expect(tracker.snapshot().size).toBe(1));
  tracker.observe([session("stopped", 4n)], projects);
  expect(tracker.snapshot().size).toBe(0);
  tracker.dispose();
});

it("does not query foreground work or invent activity when status is unavailable", async () => {
  const background = vi.fn().mockResolvedValue(reply(true, "run", true));
  const tracker = new SessionBackground({ background });
  tracker.observe([session("working")], projects);
  expect(background).not.toHaveBeenCalled();
  tracker.observe([session("idle", 2n)], projects);
  await vi.waitFor(() => expect(background).toHaveBeenCalledTimes(1));
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(tracker.snapshot().size).toBe(0);
  tracker.dispose();
});

it("waits for the resource UUID instead of falling back to a runtime handle", async () => {
  const background = vi.fn().mockResolvedValue(reply(true));
  const tracker = new SessionBackground({ background });
  const partial = session();
  partial.id = new Uint8Array();
  tracker.observe([partial], projects);
  expect(background).not.toHaveBeenCalled();
  tracker.observe([session()], projects);
  await vi.waitFor(() => expect(tracker.snapshot().has("session")).toBe(true));
  expect(background).toHaveBeenCalledTimes(1);
  tracker.dispose();
});

it("limits concurrent requests and ignores replies from an obsolete run", async () => {
  const pending: Array<{
    resolve: (value: ReturnType<typeof reply>) => void;
    signal: AbortSignal;
  }> = [];
  const background = vi.fn(
    (_req, { signal }) =>
      new Promise<ReturnType<typeof reply>>((resolve) =>
        pending.push({ resolve, signal }),
      ),
  );
  const tracker = new SessionBackground({ background: background as any });
  tracker.observe(
    Array.from({ length: 8 }, (_, i) => session("idle", 1n, "run", String(i))),
    projects,
  );
  expect(background).toHaveBeenCalledTimes(4);
  tracker.observe([session("idle", 2n, "new", "0")], projects);
  pending[0].resolve(reply(true));
  await vi.waitFor(() => expect(background).toHaveBeenCalledTimes(5));
  expect(tracker.snapshot().size).toBe(0);
  pending[4].resolve(reply(true, "new"));
  await vi.waitFor(() => expect(tracker.snapshot().has("0")).toBe(true));
  tracker.dispose();
  expect(pending.slice(1, 4).every((p) => p.signal.aborted)).toBe(true);
});

it("backs off failed snapshots across catalog updates and retries after recovery", async () => {
  vi.useFakeTimers();
  const background = vi.fn().mockRejectedValue(new Error("unavailable"));
  const tracker = new SessionBackground({ background });
  tracker.observe([session()], projects);
  await vi.advanceTimersByTimeAsync(0);
  for (let i = 2; i < 20; i++) {
    tracker.observe([session("idle", BigInt(i))], projects);
    await vi.advanceTimersByTimeAsync(0);
  }
  expect(background).toHaveBeenCalledTimes(1);
  await vi.advanceTimersByTimeAsync(30_000);
  background.mockResolvedValue(reply(true));
  tracker.observe([session("idle", 20n)], projects);
  await vi.advanceTimersByTimeAsync(0);
  expect(background).toHaveBeenCalledTimes(2);
  expect(tracker.snapshot().has("session")).toBe(true);
  tracker.dispose();
});
