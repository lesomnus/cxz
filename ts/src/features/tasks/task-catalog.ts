import { projectRef } from "#src/shared/api/project-id.ts";
import { Code, ConnectError, type Client } from "@connectrpc/connect";
import { useMemo, useSyncExternalStore } from "react";
import type { Task } from "#gen/cxz/task_pb";
import type { TaskService } from "#gen/cxz/task_svc_pb";
import { key } from "@lesomnus/payday/store";
import type { TaskCache } from "./task-cache";

type Snapshot = { tasks: Task[]; loading: boolean; error: string };
type TaskClient = Pick<Client<typeof TaskService>, "watch">;

// Consumers of a project share its subscription and cached snapshot.
export class ProjectTasks {
  private state: Snapshot;
  private rows = new Map<string, Task>();
  private listeners = new Set<() => void>();
  private controller?: AbortController;
  private cache?: TaskCache;
  private changed = false;
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
      if (this.changed) {
        value.write(this.project, [...this.rows.values()]);
        return;
      }
      const tasks = value.read(this.project);
      if (tasks) {
        this.rows = new Map(tasks.map((task) => [key(task.id), task]));
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
    this.changed = true;
    this.rows.set(key(task.id), task);
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
  private async watch(signal: AbortSignal) {
    let retry = 0;
    while (!signal.aborted) {
      let first = true;
      try {
        for await (const frame of this.client.watch(
          { project: projectRef(this.project) },
          { signal },
        )) {
          if (signal.aborted) return;
          if (first) this.rows.clear();
          first = false;
          retry = 0;
          for (const item of frame.items) {
            if (item.value) this.rows.set(key(item.id), item.value);
            else this.rows.delete(key(item.id));
          }
          this.changed = true;
          this.cache?.write(this.project, [...this.rows.values()]);
          this.publish(false);
        }
        throw new Error("Task subscription disconnected.");
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
          [Code.Unimplemented, Code.InvalidArgument, Code.NotFound].includes(
            code,
          )
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
