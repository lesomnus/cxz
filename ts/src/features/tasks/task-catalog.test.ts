import { create } from "@bufbuild/protobuf";
import { Code, ConnectError, type Client } from "@connectrpc/connect";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskSchema } from "#gen/cxz/task_pb";
import {
  TaskWatchResponseSchema,
  type TaskService,
  type TaskWatchResponse,
} from "#gen/cxz/task_svc_pb";
import { TaskCatalog } from "./task-catalog";

class Feed {
  values: TaskWatchResponse[] = [];
  wake?: () => void;
  push(value: TaskWatchResponse) {
    this.values.push(value);
    this.wake?.();
  }
  async *read(signal?: AbortSignal) {
    while (!signal?.aborted) {
      const value = this.values.shift();
      if (value) {
        yield value;
        continue;
      }
      await new Promise<void>((resolve) => {
        const done = () => {
          signal?.removeEventListener("abort", done);
          resolve();
        };
        this.wake = done;
        signal?.addEventListener("abort", done, { once: true });
      });
    }
  }
}
const task = (id: number, name = `Task ${id}`) =>
  create(TaskSchema, { id: new Uint8Array([id]), name, alias: `task-${id}` });
const frame = (...values: ReturnType<typeof task>[]) =>
  create(TaskWatchResponseSchema, {
    items: values.map((value) => ({ id: value.id, value })),
  });
const tick = async () => {
  await vi.advanceTimersByTimeAsync(0);
};
afterEach(() => vi.useRealTimers());

describe("project task subscriptions", () => {
  it("shares one watch, merges snapshot pages and changes, and cancels only after the last consumer leaves", async () => {
    vi.useFakeTimers();
    const feed = new Feed();
    const watch = vi.fn<Pick<Client<typeof TaskService>, "watch">["watch"]>(
      (_r, options) => feed.read(options?.signal),
    );
    const catalog = new TaskCatalog({ watch });
    const board = catalog.forProject("P"),
      toolbar = catalog.forProject("P");
    expect(toolbar).toBe(board);
    const offBoard = board.subscribe(() => {}),
      offToolbar = toolbar.subscribe(() => {});
    expect(watch).toHaveBeenCalledTimes(1);
    expect(watch.mock.calls[0][0]).toEqual({
      project: { key: { case: "runtimeId", value: "P" } },
    });
    feed.push(frame());
    await tick();
    expect(board.snapshot()).toMatchObject({
      loading: false,
      tasks: [],
      error: "",
    });
    feed.push(frame(task(1)));
    feed.push(frame(task(2)));
    await tick();
    expect(board.snapshot().tasks.map((task) => task.name)).toEqual([
      "Task 1",
      "Task 2",
    ]);
    feed.push(frame(task(1, "Edited by another agent")));
    await tick();
    feed.push(create(TaskWatchResponseSchema, { items: [{ id: task(2).id }] }));
    await tick();
    expect(board.snapshot().tasks.map((task) => task.name)).toEqual([
      "Edited by another agent",
    ]);
    offBoard();
    expect(watch.mock.calls[0][1]?.signal?.aborted).toBe(false);
    offToolbar();
    expect(watch.mock.calls[0][1]?.signal?.aborted).toBe(true);
    catalog.dispose();
  });

  it("keeps a warm view while reconnecting and replaces it with the new snapshot without cross-project leakage", async () => {
    vi.useFakeTimers();
    const feeds: Feed[] = [];
    const watch = vi.fn<Pick<Client<typeof TaskService>, "watch">["watch"]>(
      (_r, options) => {
        const feed = new Feed();
        feeds.push(feed);
        return feed.read(options?.signal);
      },
    );
    const catalog = new TaskCatalog({ watch });
    const p = catalog.forProject("P"),
      q = catalog.forProject("Q");
    const off = p.subscribe(() => {});
    feeds[0].push(frame(task(1), task(2)));
    await tick();
    off();
    const back = p.subscribe(() => {});
    expect(p.snapshot().tasks).toHaveLength(2);
    expect(p.snapshot().loading).toBe(true);
    const offQ = q.subscribe(() => {});
    feeds[1].push(frame(task(2, "Latest state")));
    feeds[2].push(frame(task(3)));
    await tick();
    expect(p.snapshot().tasks.map((task) => task.name)).toEqual([
      "Latest state",
    ]);
    expect(q.snapshot().tasks.map((task) => task.name)).toEqual(["Task 3"]);
    feeds[0].push(frame(task(4)));
    await tick();
    expect(p.snapshot().tasks).toHaveLength(1);
    back();
    offQ();
    catalog.dispose();
  });

  it("reports unsupported APIs without retry loops and lets the user retry after an upgrade", async () => {
    vi.useFakeTimers();
    const feed = new Feed();
    const watch = vi.fn<Pick<Client<typeof TaskService>, "watch">["watch"]>(
      () => {
        throw new ConnectError("not implemented", Code.Unimplemented);
      },
    );
    const catalog = new TaskCatalog({ watch });
    const p = catalog.forProject("P"),
      off = p.subscribe(() => {});
    await vi.advanceTimersByTimeAsync(60000);
    expect(watch).toHaveBeenCalledTimes(1);
    expect(p.snapshot()).toMatchObject({
      loading: false,
      error: expect.stringContaining("Update the Manager"),
    });
    watch.mockImplementation((_r, options) => feed.read(options?.signal));
    p.refresh();
    feed.push(frame(task(1)));
    await tick();
    expect(p.snapshot()).toMatchObject({
      loading: false,
      error: "",
      tasks: [expect.objectContaining({ name: "Task 1" })],
    });
    off();
    catalog.dispose();
  });
});
