import { create } from "@bufbuild/protobuf";
import {
  Code,
  ConnectError,
  type Client,
  type CallOptions,
} from "@connectrpc/connect";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskSchema, type Task } from "#gen/cxz/task_pb";
import {
  TaskListResponseSchema,
  TaskWatchResponseSchema,
  type TaskService,
  type TaskWatchResponse,
} from "#gen/cxz/task_svc_pb";
import { TaskCatalog } from "./task-catalog";
import { TaskCache } from "./task-cache";
import { Store } from "@lesomnus/payday/store";
import { Task as TaskEntity, Project } from "#gen/entities";

type TaskClient = Pick<Client<typeof TaskService>, "watch" | "list">;
const task = (id: number, name = `Task ${id}`, version = 1) =>
  create(TaskSchema, {
    id: new Uint8Array([id]),
    name,
    alias: `task-${id}`,
    dateUpdated: { seconds: BigInt(version) },
  });
const frame = (...values: Task[]) =>
  create(TaskWatchResponseSchema, {
    items: values.map((value) => ({ id: value.id, value })),
  });
const page = (items: Task[], next = "") =>
  create(TaskListResponseSchema, { items, next });
const deferred = <T>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
};
class Feed {
  values: (TaskWatchResponse | Error)[] = [];
  wake?: () => void;
  constructor(readonly options?: CallOptions) {}
  ready(valid = true) {
    this.options?.onHeader?.(
      new Headers(valid ? { "cxz-watch-ready": "1" } : {}),
    );
  }
  push(value: TaskWatchResponse | Error) {
    this.values.push(value);
    this.wake?.();
  }
  async *read() {
    const signal = this.options?.signal;
    while (!signal?.aborted) {
      const value = this.values.shift();
      if (value instanceof Error) throw value;
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
        if (signal?.aborted) done();
      });
    }
  }
}
function client(
  list: TaskClient["list"] = async () => page([]),
  automaticReady = true,
) {
  const feeds: Feed[] = [];
  return {
    feeds,
    list: vi.fn<TaskClient["list"]>(list),
    watch: vi.fn<TaskClient["watch"]>((_request, options) => {
      const feed = new Feed(options);
      feeds.push(feed);
      if (automaticReady) feed.ready();
      return feed.read();
    }),
  };
}
const cache = () =>
  new TaskCache(
    Store.open([TaskEntity, Project], { name: "test", identity: "reader" }),
  );
const tick = () => vi.advanceTimersByTimeAsync(0);
afterEach(() => vi.useRealTimers());

describe("project Task List and changes-only Watch", () => {
  it("waits for readiness, keeps cached membership through all pages, and version-merges buffered changes atomically", async () => {
    vi.useFakeTimers();
    const saved = cache();
    saved.write("P", [task(1), task(2)]);
    saved.write("Q", [task(9)]);
    const last = deferred<ReturnType<typeof page>>();
    const api = client(
      async (r) =>
        r.after
          ? last.promise
          : page([task(1), task(2, "List is newest", 4)], "next"),
      false,
    );
    const catalog = new TaskCatalog(api, Promise.resolve(saved));
    const project = catalog.forProject("P");
    const listener = vi.fn();
    project.subscribe(listener);
    await tick();
    expect(project.snapshot().tasks.map((t) => t.name)).toEqual([
      "Task 1",
      "Task 2",
    ]);
    expect(api.list).not.toHaveBeenCalled();
    api.feeds[0].ready();
    await tick();
    expect(api.list).toHaveBeenCalledTimes(2);
    expect(api.list.mock.calls[0][0]).toEqual({
      project: { key: { case: "alias", value: "P" } },
      reverse: true,
      size: 500,
      after: undefined,
    });
    api.feeds[0].push(frame(task(2, "Older buffered update", 3), task(4)));
    api.feeds[0].push(
      create(TaskWatchResponseSchema, { items: [{ id: task(1).id }] }),
    );
    await tick();
    expect(saved.read("P")?.map((t) => t.name)).toEqual(["Task 1", "Task 2"]);
    listener.mockClear();
    last.resolve(page([task(3)]));
    await tick();
    expect(listener).toHaveBeenCalledTimes(1);
    expect(project.snapshot().tasks.map((t) => t.name)).toEqual([
      "List is newest",
      "Task 3",
      "Task 4",
    ]);
    expect(saved.read("P")?.map((t) => t.name)).toEqual([
      "List is newest",
      "Task 3",
      "Task 4",
    ]);
    expect(saved.read("Q")?.map((t) => t.name)).toEqual(["Task 9"]);
    expect(api.watch.mock.calls[0][0]).toEqual({
      project: { key: { case: "alias", value: "P" } },
      skipSnapshot: true,
    });
    catalog.dispose();
  });

  it("does not expose cold partial pages or lose a newer Watch row to List", async () => {
    vi.useFakeTimers();
    const last = deferred<ReturnType<typeof page>>();
    const api = client(async (r) =>
      r.after ? last.promise : page([task(1)], "last"),
    );
    const catalog = new TaskCatalog(api);
    const project = catalog.forProject("P");
    project.subscribe(() => {});
    await tick();
    const changed = task(1, "Changed during List");
    changed.dateUpdated!.nanos = 500;
    api.feeds[0].push(frame(changed));
    await tick();
    expect(project.snapshot()).toMatchObject({ loading: true, tasks: [] });
    last.resolve(page([task(2)]));
    await tick();
    expect(project.snapshot()).toMatchObject({
      loading: false,
      tasks: [{ name: "Changed during List" }, { name: "Task 2" }],
    });
    const stale = task(1, "Stale delivery");
    stale.dateUpdated!.nanos = 100;
    api.feeds[0].push(frame(stale));
    await tick();
    expect(project.snapshot().tasks[0].name).toBe("Changed during List");
    api.feeds[0].push(
      create(TaskWatchResponseSchema, { items: [{ id: task(2).id }] }),
    );
    await tick();
    expect(project.snapshot().tasks).toHaveLength(1);
    catalog.dispose();
  });

  it("keeps a local write through pending List and late disk hydration", async () => {
    vi.useFakeTimers();
    const saved = cache();
    saved.write("P", [task(1), task(2)]);
    const disk = deferred<TaskCache>();
    const listing = deferred<ReturnType<typeof page>>();
    const api = client(async () => listing.promise);
    const catalog = new TaskCatalog(api, disk.promise);
    const project = catalog.forProject("P");
    project.subscribe(() => {});
    project.apply(task(3, "Local write", 5));
    disk.resolve(saved);
    await tick();
    expect(project.snapshot().tasks.map((t) => t.name)).toEqual([
      "Task 1",
      "Task 2",
      "Local write",
    ]);
    listing.resolve(page([task(1)]));
    await tick();
    expect(project.snapshot().tasks.map((t) => t.name)).toEqual([
      "Task 1",
      "Local write",
    ]);
    catalog.dispose();
  });

  it("never restores stale disk data after a complete List has committed", async () => {
    vi.useFakeTimers();
    const saved = cache();
    saved.write("P", [task(1, "Old disk value")]);
    const disk = deferred<TaskCache>();
    const api = client(async () => page([task(1, "Fresh List", 5)]));
    const catalog = new TaskCatalog(api, disk.promise);
    const project = catalog.forProject("P");
    project.subscribe(() => {});
    await tick();
    disk.resolve(saved);
    await tick();
    expect(project.snapshot().tasks[0].name).toBe("Fresh List");
    expect(saved.read("P")?.[0].name).toBe("Fresh List");
    catalog.dispose();
  });

  it("shares a project subscription and cancels both calls only after its last consumer leaves", async () => {
    vi.useFakeTimers();
    const listing = deferred<ReturnType<typeof page>>();
    const api = client(async () => listing.promise);
    const catalog = new TaskCatalog(api);
    const p = catalog.forProject("P");
    expect(catalog.forProject("P")).toBe(p);
    const first = p.subscribe(() => {}),
      second = p.subscribe(() => {});
    await tick();
    expect(api.watch).toHaveBeenCalledTimes(1);
    expect(api.list).toHaveBeenCalledTimes(1);
    first();
    expect(api.feeds[0].options?.signal?.aborted).toBe(false);
    second();
    expect(api.feeds[0].options?.signal?.aborted).toBe(true);
    expect(api.list.mock.calls[0][1]?.signal?.aborted).toBe(true);
    listing.resolve(page([task(1)]));
    await tick();
    expect(p.snapshot().tasks).toEqual([]);
    catalog.dispose();
  });

  it("keeps a warm view on reconnect, resyncs with List and ignores an abandoned response", async () => {
    vi.useFakeTimers();
    const abandoned = deferred<ReturnType<typeof page>>();
    let reads = 0;
    const api = client(async () =>
      ++reads === 1
        ? page([task(1), task(2)])
        : reads === 2
          ? abandoned.promise
          : page([task(2, "Reconnected", 4)]),
    );
    const catalog = new TaskCatalog(api);
    const p = catalog.forProject("P");
    p.subscribe(() => {});
    await tick();
    p.refresh();
    await tick();
    expect(p.snapshot().tasks).toHaveLength(2);
    api.feeds[1].push(new ConnectError("Disconnected", Code.Unavailable));
    await tick();
    expect(api.list.mock.calls[1][1]?.signal?.aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(500);
    expect(p.snapshot().tasks.map((t) => t.name)).toEqual(["Reconnected"]);
    abandoned.resolve(page([task(1, "Too late", 99)]));
    await tick();
    expect(p.snapshot().tasks.map((t) => t.name)).toEqual(["Reconnected"]);
    catalog.dispose();
  });

  it("reports old runtimes without listing or retrying until explicitly refreshed", async () => {
    vi.useFakeTimers();
    const api = client(async () => page([task(1)]), false);
    const catalog = new TaskCatalog(api);
    const p = catalog.forProject("P");
    p.subscribe(() => {});
    api.feeds[0].ready(false);
    await vi.advanceTimersByTimeAsync(60000);
    expect(api.watch).toHaveBeenCalledTimes(1);
    expect(api.list).not.toHaveBeenCalled();
    expect(p.snapshot().error).toContain("Update the Manager");
    p.refresh();
    api.feeds[1].ready();
    await tick();
    expect(p.snapshot()).toMatchObject({
      error: "",
      loading: false,
      tasks: [{ name: "Task 1" }],
    });
    catalog.dispose();
  });

  it("settles an empty project from List and keeps changes isolated between projects", async () => {
    vi.useFakeTimers();
    const api = client(async (r) =>
      r.project?.key?.value === "Q" ? page([task(9)]) : page([]),
    );
    const catalog = new TaskCatalog(api, Promise.resolve(undefined));
    const p = catalog.forProject("P"),
      q = catalog.forProject("Q");
    p.subscribe(() => {});
    q.subscribe(() => {});
    await tick();
    expect(p.snapshot()).toMatchObject({ tasks: [], loading: false });
    expect(q.snapshot().tasks[0].name).toBe("Task 9");
    api.feeds[0].push(frame(task(1)));
    await tick();
    expect(p.snapshot().tasks[0].name).toBe("Task 1");
    expect(q.snapshot().tasks[0].name).toBe("Task 9");
    catalog.dispose();
    catalog.forProject("P").apply(task(3));
    expect(catalog.forProject("P").snapshot().tasks).toEqual([]);
  });

  it("bounds a quiet server that never sends subscription headers", async () => {
    vi.useFakeTimers();
    const api = client(undefined, false);
    const catalog = new TaskCatalog(api);
    const p = catalog.forProject("P");
    p.subscribe(() => {});
    await vi.advanceTimersByTimeAsync(15_000);
    expect(p.snapshot()).toMatchObject({ loading: false, tasks: [] });
    expect(p.snapshot().error).toContain("Update the Manager");
    expect(api.feeds[0].options?.signal?.aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(api.watch).toHaveBeenCalledTimes(1);
    expect(api.list).not.toHaveBeenCalled();
    catalog.dispose();
  });

  it("reports an unsupported Watch without waiting for readiness", async () => {
    vi.useFakeTimers();
    const api = client(undefined, false);
    const catalog = new TaskCatalog(api);
    const p = catalog.forProject("P");
    p.subscribe(() => {});
    api.feeds[0].push(new ConnectError("Not implemented", Code.Unimplemented));
    await tick();
    expect(p.snapshot().error).toContain("Task API unavailable");
    expect(p.snapshot().loading).toBe(false);
    expect(api.list).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
    catalog.dispose();
  });
});
