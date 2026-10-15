import { Code, ConnectError, type Client } from "@connectrpc/connect";
import { useMemo, useSyncExternalStore } from "react";
import type { Task } from "#gen/cxz/task_pb";
import type { TaskService } from "#gen/cxz/task_svc_pb";
import { key } from "@lesomnus/payday/store";

type Snapshot = { tasks: Task[]; loading: boolean; error: string };
type TaskClient = Pick<Client<typeof TaskService>, "watch">;

// Consumers of a project share its subscription and cached snapshot.
export class ProjectTasks {
  private state: Snapshot;
  private rows = new Map<string, Task>();
  private listeners = new Set<() => void>();
  private controller?: AbortController;
  constructor(
    private client: TaskClient,
    readonly project: string,
  ) {
    this.state = { tasks: [], loading: !!project, error: "" };
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
    this.rows.set(key(task.id), task);
    this.publish(false, this.state.error);
  }
  refresh = () => {
    this.stop();
    if (!this.listeners.size || !this.project) return;
    const controller = new AbortController();
    this.controller = controller;
    this.publish(true);
    void this.watch(controller.signal);
  };
  stop() {
    this.controller?.abort();
    this.controller = undefined;
  }
  private async watch(signal: AbortSignal) {
    let retry = 0;
    while (!signal.aborted) {
      let first = true;
      try {
        for await (const frame of this.client.watch(
          { project: { key: { case: "runtimeId", value: this.project } } },
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
  constructor(private client: TaskClient) {}
  forProject(project: string) {
    let state = this.projects.get(project);
    if (!state) {
      state = new ProjectTasks(this.client, project);
      this.projects.set(project, state);
    }
    return state;
  }
  dispose() {
    for (const state of this.projects.values()) state.stop();
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
