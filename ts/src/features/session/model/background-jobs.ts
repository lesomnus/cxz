import type { SessionEvent } from "#gen/cxz/session_pb";
import { payload } from "./journal";
import {
  backgroundStateTasks,
  type BackgroundTask,
} from "#src/features/activity/background-tasks.ts";

const kinds = new Set([
  "background_tasks_changed",
  "task_started",
  "task_updated",
  "task_notification",
  "background_status",
]);
const finished = new Set([
  "completed",
  "failed",
  "stopped",
  "cancelled",
  "canceled",
]);
// The counter needs no output paths or potentially large completion summaries.
const fields = ["task_id", "status"] as const;

// Match agentview.BackgroundState: snapshots are authoritative, foreground
// starts and unknown task updates must not invent background work.
export class BackgroundJobs {
  private tasks = new Map<string, Partial<BackgroundTask>>();
  private lastSeq = 0n;
  private collecting = true;
  private snapshotStart = 0n;
  private pending: SessionEvent[] = [];
  private value: { count?: number } = {};
  private available = false;
  private listeners = new Set<() => void>();
  constructor(readonly run: string) {}
  snapshot = () => this.value;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  beginSnapshot() {
    this.collecting = true;
    this.snapshotStart = this.lastSeq;
    this.pending = [];
  }
  private publish() {
    const count = this.available
      ? [...this.tasks.values()].filter((task) => task.active === true).length
      : undefined;
    if (this.value.count === count) return;
    this.value = { count };
    this.listeners.forEach((listener) => listener());
  }
  seed(bytes: Uint8Array, seq: bigint) {
    if (seq < this.snapshotStart) {
      this.cancelSnapshot();
      return;
    }
    const states = JSON.parse(new TextDecoder().decode(bytes) || "{}");
    if (states[this.run]) {
      this.available = states[this.run].Unavailable !== true;
      this.tasks = new Map(
        backgroundStateTasks(states[this.run]).map((task) => [
          task.task_id,
          { task_id: task.task_id, active: task.active, status: task.status },
        ]),
      );
      this.publish();
    }
    this.lastSeq = seq;
    const newer = this.pending
      .filter((event) => event.seq > seq)
      .sort((a, b) => (a.seq < b.seq ? -1 : a.seq > b.seq ? 1 : 0));
    this.pending = [];
    this.collecting = false;
    for (const event of newer) this.receive(event);
  }
  cancelSnapshot() {
    this.collecting = false;
    this.pending = [];
  }
  receive = (event: SessionEvent) => {
    if (
      event.kind !== "background" ||
      event.runId !== this.run ||
      event.seq <= this.lastSeq
    )
      return;
    const data = payload(event);
    const native =
      data.type === "system" && data.subtype !== "background_status";
    const codex =
      data.type === "background" &&
      data.provider === "codex" &&
      ["background_tasks_changed", "background_status"].includes(data.subtype);
    if ((!native && !codex) || !kinds.has(data.subtype)) return;
    this.lastSeq = event.seq;
    if (this.collecting) this.pending.push(event);
    if (data.subtype === "background_status") {
      this.available = false;
      this.publish();
      return;
    }
    const merge = (
      target: Partial<BackgroundTask>,
      source: Record<string, unknown>,
    ) => {
      for (const key of fields)
        if (typeof source[key] === "string" && source[key])
          (target as Record<string, unknown>)[key] = source[key];
    };
    if (data.subtype === "background_tasks_changed") {
      if (!Array.isArray(data.tasks)) return;
      for (const task of this.tasks.values()) task.active = false;
      for (const incoming of data.tasks) {
        if (
          !incoming ||
          typeof incoming.task_id !== "string" ||
          !incoming.task_id
        )
          continue;
        const task = this.tasks.get(incoming.task_id) ?? {};
        merge(task, incoming);
        task.active = true;
        task.status = "working";
        this.tasks.set(incoming.task_id, task);
      }
    } else {
      if (typeof data.task_id !== "string" || !data.task_id) return;
      const task = this.tasks.get(data.task_id);
      if (
        !task &&
        !(data.subtype === "task_started" && data.is_backgrounded === true)
      )
        return;
      const next = task ?? {};
      merge(next, data);
      if (data.subtype === "task_started" && data.is_backgrounded === true) {
        next.active = true;
        next.status = "working";
      }
      if (
        data.subtype === "task_updated" &&
        data.patch &&
        typeof data.patch === "object"
      )
        merge(next, data.patch);
      if (next.status && finished.has(next.status)) next.active = false;
      this.tasks.set(data.task_id, next);
    }
    this.available = true;
    this.publish();
  };
}
