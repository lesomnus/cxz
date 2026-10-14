import type { AuxState } from "#gen/cxz/session_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ref } from "#src/shared/api/connection.ts";

// AuxEvents sends complete snapshots, including summaries already persisted
// by the manager. Keep the last snapshot while reconnecting or changing views;
// only a new authoritative snapshot may replace it.
export class CachedSessionAux {
  private state?: AuxState;
  private listeners = new Set<() => void>();
  private controller?: AbortController;
  private timer?: ReturnType<typeof setTimeout>;
  private started = false;
  private paused = false;
  private stopped = false;
  constructor(
    private sessions: Connection["sessions"],
    private id: string,
    private changed: () => void,
  ) {}
  snapshot = () => this.state;
  get viewed() {
    return this.listeners.size > 0;
  }
  get active() {
    return ["queued", "running"].includes(this.state?.current?.state ?? "");
  }
  get bytes() {
    if (!this.state) return 0;
    const s = this.state;
    const tasks = [...s.recent, ...(s.current ? [s.current] : [])];
    return (
      2 * (s.title.length + s.message.length) +
      s.summaries.reduce(
        (n, summary) =>
          n + 2 * (summary.text.length + summary.runId.length) + 64,
        0,
      ) +
      tasks.reduce(
        (n, task) =>
          n +
          2 *
            (task.message.length +
              task.runId.length +
              task.results.reduce((n, result) => n + result.text.length, 0)) +
          task.usage.reduce(
            (n, usage) =>
              n +
              usage.data.byteLength +
              2 * (usage.model.length + usage.account.length),
            0,
          ) +
          128,
        0,
      )
    );
  }
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    if (!this.stopped && this.id) {
      if (this.paused) this.resume();
      if (!this.started) {
        this.started = true;
        void this.connect();
      }
    }
    return () => {
      this.listeners.delete(listener);
      // Releasing the view does not close the stream. The conversation's LRU
      // owns its lifetime, including aux-only views and in-flight aux tasks.
      this.changed();
    };
  };
  private publish(next: AuxState | undefined) {
    this.state = next;
    for (const listener of this.listeners) listener();
    // Even a completed snapshot received offscreen can grow the idle cache.
    if (!this.viewed && !this.active) this.changed();
  }
  private async connect() {
    if (this.stopped || this.paused) return;
    const controller = new AbortController();
    this.controller = controller;
    try {
      for await (const next of this.sessions.auxEvents(
        { ref: ref(this.id) },
        { signal: controller.signal },
      )) {
        if (controller.signal.aborted || this.controller !== controller) return;
        this.publish(next);
      }
    } catch {
      // Aux being unavailable must not interrupt reading the conversation.
    } finally {
      if (!this.stopped && !this.paused && this.controller === controller)
        this.timer = setTimeout(() => void this.connect(), 5000);
    }
  }
  reconnect() {
    if (!this.started || this.paused || this.stopped) return;
    clearTimeout(this.timer);
    this.controller?.abort();
    void this.connect();
  }
  pause() {
    this.paused = true;
    clearTimeout(this.timer);
    this.controller?.abort();
  }
  resume() {
    if (!this.paused || this.stopped) return;
    this.paused = false;
    if (this.started) void this.connect();
  }
  reset() {
    this.publish(undefined);
    this.reconnect();
  }
  dispose() {
    this.stopped = true;
    this.pause();
    this.listeners.clear();
    this.state = undefined;
  }
}
