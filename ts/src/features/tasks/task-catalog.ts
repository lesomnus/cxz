import { projectRef } from "#src/shared/api/project-id.ts";
import { Code, ConnectError, type Client } from "@connectrpc/connect";
import { useMemo, useSyncExternalStore } from "react";
import type { Task } from "#gen/cxz/task_pb";
import type { TaskService } from "#gen/cxz/task_svc_pb";
import { key } from "@lesomnus/payday/store";
import type { TaskCache } from "./task-cache";

type Snapshot = { tasks: Task[]; loading: boolean; error: string };
type TaskClient = Pick<Client<typeof TaskService>, "watch" | "list">;
type Changes = Map<string, Task | undefined>;

function compareVersion(a: Task, b: Task) {
  const seconds =
    (a.dateUpdated?.seconds ?? 0n) - (b.dateUpdated?.seconds ?? 0n);
  return seconds === 0n
    ? (a.dateUpdated?.nanos ?? 0) - (b.dateUpdated?.nanos ?? 0)
    : seconds > 0n
      ? 1
      : -1;
}
function upsert(rows: Map<string, Task>, task: Task) {
  const id = key(task.id),
    previous = rows.get(id);
  if (!previous || compareVersion(task, previous) >= 0) rows.set(id, task);
}
function bufferChange(changes: Changes, id: string, task: Task | undefined) {
  const previous = changes.get(id);
  if (!task || !previous || compareVersion(task, previous) >= 0)
    changes.set(id, task);
}

// Consumers of a project share its subscription and cached snapshot.
export class ProjectTasks {
  private state: Snapshot;
  private rows = new Map<string, Task>();
  private listeners = new Set<() => void>();
  private controller?: AbortController;
  private cache?: TaskCache;
  private complete = false;
  private pending?: Changes;
  private disposed = false;
  constructor(
    private client: TaskClient,
    readonly project: string,
    cache?: Promise<TaskCache | undefined>,
  ) {
    this.state = { tasks: [], loading: !!project, error: "" };
    void cache?.then((value) => {
      if (this.disposed || !value) return;
      this.cache = value;
      if (this.complete) {
        value.write(this.project, [...this.rows.values()]);
        return;
      }
      const tasks = value.read(this.project);
      if (tasks) {
        const hydrated = new Map(tasks.map((task) => [key(task.id), task]));
        for (const task of this.rows.values()) upsert(hydrated, task);
        this.rows = hydrated;
        this.publish(false, this.state.error);
      }
    });
  }
  snapshot = () => this.state;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    if (!this.controller && this.project) this.refresh();
    return () => {
      this.listeners.delete(listener);
      if (!this.listeners.size) this.stop();
    };
  };
  private publish(loading: boolean, error = "") {
    this.state = { tasks: [...this.rows.values()], loading, error };
    this.listeners.forEach((listener) => listener());
  }
  apply(task: Task) {
    if (this.disposed) return;
    upsert(this.rows, task);
    if (this.pending) bufferChange(this.pending, key(task.id), task);
    this.cache?.write(this.project, [...this.rows.values()]);
    this.publish(false, this.state.error);
  }
  refresh = () => {
    this.stop();
    if (this.disposed || !this.listeners.size || !this.project) return;
    const controller = new AbortController();
    this.controller = controller;
    this.publish(true);
    void this.watch(controller.signal);
  };
  stop() {
    this.controller?.abort();
    this.controller = undefined;
  }
  dispose() {
    this.disposed = true;
    this.stop();
  }
  private async reconcile(
    signal: AbortSignal,
    synchronized: () => void,
  ): Promise<never> {
    const controller = new AbortController();
    const abort = () => controller.abort();
    signal.addEventListener("abort", abort, { once: true });
    if (signal.aborted) abort();
    const changes: Changes = new Map();
    this.pending = changes;
    let syncing = true;
    let ready!: () => void, rejectReady!: (error: unknown) => void;
    const subscriptionReady = new Promise<void>((resolve, reject) => {
      ready = resolve;
      rejectReady = reject;
    });
    const readinessError = () =>
      new ConnectError(
        "Task Watch did not confirm its subscription. Update the Manager and project runtime, then retry.",
        Code.FailedPrecondition,
      );
    const timer = setTimeout(() => rejectReady(readinessError()), 15_000);
    const watching = (async (): Promise<never> => {
      for await (const frame of this.client.watch(
        { project: projectRef(this.project), skipSnapshot: true },
        {
          signal: controller.signal,
          timeoutMs: 0,
          onHeader: (headers) => {
            clearTimeout(timer);
            if (headers.get("cxz-watch-ready") !== "1")
              rejectReady(readinessError());
            else ready();
          },
        },
      )) {
        if (controller.signal.aborted)
          throw new ConnectError("Task subscription canceled", Code.Canceled);
        for (const item of frame.items) {
          if (syncing) bufferChange(changes, key(item.id), item.value);
          else if (item.value) upsert(this.rows, item.value);
          else this.rows.delete(key(item.id));
        }
        if (!syncing && frame.items.length) {
          this.cache?.write(this.project, [...this.rows.values()]);
          this.publish(false);
        }
      }
      throw new ConnectError(
        "Task subscription disconnected.",
        Code.Unavailable,
      );
    })();
    // The watcher remains live while List is pending and after it has committed.
    void watching.catch(() => {});
    try {
      await Promise.race([subscriptionReady, watching]);
      const listing = (async () => {
        const rows = new Map<string, Task>();
        const cursors = new Set<string>();
        let after: string | undefined;
        do {
          const page = await this.client.list(
            {
              project: projectRef(this.project),
              reverse: true,
              size: 500,
              after,
            },
            { signal: controller.signal, timeoutMs: 15_000 },
          );
          for (const task of page.items) upsert(rows, task);
          after = page.next || undefined;
          if (after && cursors.has(after))
            throw new ConnectError(
              "Task List repeated its pagination cursor.",
              Code.DataLoss,
            );
          if (after) cursors.add(after);
        } while (after);
        return rows;
      })();
      const rows = await Promise.race([listing, watching]);
      if (controller.signal.aborted)
        throw new ConnectError("Task synchronization canceled", Code.Canceled);
      for (const [id, task] of changes) {
        if (task) upsert(rows, task);
        else rows.delete(id);
      }
      this.rows = rows;
      this.complete = true;
      syncing = false;
      this.pending = undefined;
      this.cache?.write(this.project, [...this.rows.values()]);
      this.publish(false);
      synchronized();
      return await watching;
    } finally {
      clearTimeout(timer);
      controller.abort();
      signal.removeEventListener("abort", abort);
      if (this.pending === changes) this.pending = undefined;
    }
  }

  private async watch(signal: AbortSignal) {
    let retry = 0;
    while (!signal.aborted) {
      try {
        await this.reconcile(signal, () => {
          retry = 0;
        });
      } catch (error) {
        if (signal.aborted) return;
        const code = ConnectError.from(error).code;
        this.publish(
          false,
          code === Code.Unimplemented
            ? "Task API unavailable. Update the Manager and project runtime."
            : String(error),
        );
        if (
          [
            Code.Unimplemented,
            Code.InvalidArgument,
            Code.NotFound,
            Code.FailedPrecondition,
          ].includes(code)
        )
          return;
      }
      const delay = Math.min(30000, 500 * 2 ** Math.min(retry++, 6));
      await new Promise<void>((resolve) => {
        const done = () => {
          clearTimeout(timer);
          signal.removeEventListener("abort", done);
          resolve();
        };
        const timer = setTimeout(done, delay);
        signal.addEventListener("abort", done, { once: true });
        if (signal.aborted) done();
      });
    }
  }
}

export class TaskCatalog {
  private projects = new Map<string, ProjectTasks>();
  private disposed = false;
  constructor(
    private client: TaskClient,
    private cache?: Promise<TaskCache | undefined>,
  ) {}
  forProject(project: string) {
    let state = this.projects.get(project);
    if (!state) {
      state = new ProjectTasks(this.client, project, this.cache);
      if (this.disposed) state.dispose();
      this.projects.set(project, state);
    }
    return state;
  }
  dispose() {
    this.disposed = true;
    for (const state of this.projects.values()) state.dispose();
    this.projects.clear();
  }
}

export function useProjectTasks(
  c: { taskCatalog: TaskCatalog },
  project: string,
) {
  const state = useMemo(() => c.taskCatalog.forProject(project), [c, project]);
  return {
    ...useSyncExternalStore(state.subscribe, state.snapshot),
    refresh: state.refresh,
  };
}
