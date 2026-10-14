import type { Session, SessionEvent } from "#gen/cxz/session_pb";
import type { Client } from "@connectrpc/connect";
import type { SessionService } from "#gen/cxz/session_svc_pb";
import { questions } from "./journal";

export type SessionAlert = "complete" | "question" | "error";
type Activity = {
  run: string;
  last: bigint;
  done: bigint;
  failed: bigint;
  seen: bigint;
  notified: bigint;
  pending: Set<string>;
};
const working = (state: string) =>
  ["working", "running", "waiting_input", "waiting"].includes(state);

// Consume the shared resource catalog, never open a live stream per session.
// Confirm turn outcomes against native history; diagnostics/settings are silent.
export class SessionAttention {
  private activities = new Map<string, Activity>();
  private restored = new Set<string>();
  private listeners = new Set<() => void>();
  private alerts = new Set<(kind: SessionAlert) => void>();
  private checks = new Map<string, AbortController>();
  private queued = new Map<string, Session>();
  private reading = { id: "", through: 0n };
  private value = new Set<string>();
  private errors = new Set<string>();
  private storageKey: string;
  private stopped = false;
  constructor(
    private api: Pick<Client<typeof SessionService>, "history">,
    scope: string,
    private storage: () => Storage = () => sessionStorage,
  ) {
    this.storageKey = `cxz.attention.v1:${encodeURIComponent(scope)}`;
    try {
      for (const [id, a] of Object.entries(
        JSON.parse(this.storage().getItem(this.storageKey) ?? "{}"),
      ) as [string, Record<string, string>][]) {
        this.activities.set(id, {
          run: a.run,
          last: BigInt(a.last),
          done: BigInt(a.done),
          failed: BigInt(a.failed ?? "0"),
          seen: BigInt(a.seen),
          notified: BigInt(a.notified),
          pending: new Set(JSON.parse(a.pending)),
        });
        this.restored.add(id);
      }
    } catch {
      this.activities.clear();
    }
    this.publish();
  }
  snapshot = () => this.value;
  errorSnapshot = () => this.errors;
  subscribe = (fn: () => void) => {
    this.listeners.add(fn);
    return () => {
      this.listeners.delete(fn);
    };
  };
  onAlert = (fn: (kind: SessionAlert) => void) => {
    this.alerts.add(fn);
    return () => {
      this.alerts.delete(fn);
    };
  };
  private publish() {
    const next = new Set(
      [...this.activities]
        .filter(([, a]) => a.done > a.seen || a.failed > a.seen)
        .map(([id]) => id),
    );
    const errors = new Set(
      [...this.activities]
        .filter(([, a]) => a.failed > a.seen)
        .map(([id]) => id),
    );
    if (
      errors.size !== this.errors.size ||
      [...errors].some((id) => !this.errors.has(id)) ||
      next.size !== this.value.size ||
      [...next].some((id) => !this.value.has(id))
    ) {
      this.value = next;
      this.errors = errors;
      this.listeners.forEach((fn) => fn());
    }
    try {
      this.storage().setItem(
        this.storageKey,
        JSON.stringify(
          Object.fromEntries(
            [...this.activities].map(([id, a]) => [
              id,
              {
                run: a.run,
                last: String(a.last),
                done: String(a.done),
                failed: String(a.failed),
                seen: String(a.seen),
                notified: String(a.notified),
                pending: JSON.stringify([...a.pending]),
              },
            ]),
          ),
        ),
      );
    } catch {
      /* optional browser storage */
    }
  }
  setReading(id: string, through = 0n) {
    this.reading = { id, through };
    const a = this.activities.get(id);
    if (a && a.seen < through) {
      a.seen = through;
      this.publish();
    }
  }
  private complete(
    id: string,
    a: Activity,
    seq: bigint,
    kind: "complete" | "error" = "complete",
  ) {
    if (seq <= a.notified) return;
    a.notified = seq;
    if (kind === "error") a.failed = seq;
    else a.done = seq;
    if (this.reading.id === id && this.reading.through >= seq) a.seen = seq;
    this.publish();
    this.alerts.forEach((fn) => fn(kind));
  }
  event(id: string, event: SessionEvent) {
    const a = this.activities.get(id);
    if (!a || a.run !== event.runId) return;
    if (event.kind === "turn_end") {
      if (event.text === "completed") this.complete(id, a, event.seq);
      else if (event.text === "failed")
        this.complete(id, a, event.seq, "error");
    }
  }
  observe(sessions: Session[]) {
    if (this.stopped) return;
    for (const s of sessions) {
      const status = s.status;
      if (!status) continue;
      const id = s.runtimeId;
      const pending = new Set(
        status.pending
          .filter(
            (e) =>
              (!e.runId || e.runId === status.runId) &&
              questions(s.agent, e).length > 0,
          )
          .map((e) => `${e.runId}/${e.requestId || e.seq}`),
      );
      let a = this.activities.get(id);
      if (!a) {
        // Initial snapshots establish a baseline instead of replaying old alerts.
        a = {
          run: status.runId,
          last: status.lastSeq,
          done: 0n,
          failed: 0n,
          seen: status.lastSeq,
          notified: status.lastSeq,
          pending,
        };
        this.activities.set(id, a);
        continue;
      }
      const restored = this.restored.delete(id);
      if (status.lastSeq < a.last) {
        if (restored) {
          // A purge or an ephemeral sandbox reset can restart its journal.
          a.last = a.seen = a.notified = status.lastSeq;
          a.done = a.failed = 0n;
          a.run = status.runId;
          a.pending = pending;
        }
        continue;
      }
      if (a.run !== status.runId) {
        this.queued.delete(id);
        this.checks.get(id)?.abort();
        this.checks.delete(id);
        a.run = status.runId;
        a.pending.clear();
      }
      if ([...pending].some((key) => !a!.pending.has(key)))
        this.alerts.forEach((fn) => fn("question"));
      a.pending = pending;
      if (
        ["idle", "failed", "error", "stopped"].includes(status.state) &&
        status.lastSeq > a.last &&
        status.lastSeq > a.notified
      ) {
        this.queued.set(id, s);
      } else if (
        !this.checks.has(id) &&
        !this.queued.has(id) &&
        working(status.state)
      ) {
        // Keep the starting cursor for the completion check at the next idle.
        a.last = status.lastSeq;
      }
    }
    this.publish();
    this.pump();
  }
  private pump() {
    if (this.stopped) return;
    for (const [id, session] of this.queued) {
      if (this.checks.size >= 4) break;
      if (this.checks.has(id)) continue;
      this.queued.delete(id);
      const controller = new AbortController();
      this.checks.set(id, controller);
      void this.check(session, controller).finally(() => {
        if (this.checks.get(id) === controller) this.checks.delete(id);
        this.pump();
      });
    }
  }
  private async check(s: Session, controller: AbortController) {
    const id = s.runtimeId,
      run = s.status!.runId,
      end = s.status!.lastSeq;
    const a = this.activities.get(id)!;
    const from = a.last;
    const timeout = setTimeout(() => controller.abort(), 5000);
    try {
      // Usually one request near the tail; older servers may page more narrowly.
      let done = 0n;
      let failed = 0n;
      for (let before = end; before > from; ) {
        const floor = before - from > 512n ? before - 512n : from;
        let cursor = floor;
        while (cursor < before) {
          const batch = await this.api.history(
            {
              ref: { key: { case: "runtimeId", value: id } },
              afterSeq: cursor,
              limit: 512,
            },
            { signal: controller.signal },
          );
          if (controller.signal.aborted || a.run !== run) return;
          if (!batch.events.length) break;
          let next = cursor;
          for (const e of batch.events) {
            if (e.seq > next) next = e.seq;
            if (
              e.runId === run &&
              e.seq > from &&
              e.seq <= end &&
              e.kind === "turn_end" &&
              ["completed", "failed"].includes(e.text)
            ) {
              if (e.text === "failed" && e.seq > failed) failed = e.seq;
              if (e.text === "completed" && e.seq > done) done = e.seq;
            }
          }
          if (next <= cursor) break;
          cursor = next;
        }
        // Retain unseen failures even when a later turn succeeded. Scan the
        // entire new range, including earlier pages, before publishing.
        before = floor;
      }
      if (!controller.signal.aborted && a.run === run) {
        if (failed > a.failed) a.failed = failed;
        if (done > a.done) a.done = done;
        const outcome = a.failed > a.notified ? "error" : "complete";
        const latest = a.failed > a.done ? a.failed : a.done;
        if (latest > a.notified) {
          a.notified = latest;
          if (this.reading.id === id && this.reading.through >= latest)
            a.seen = latest;
          this.publish();
          this.alerts.forEach((fn) => fn(outcome));
        }
        a.last = end;
        this.publish();
      }
    } catch {
      /* A subsequent catalog update can retry without a false alarm. */
    } finally {
      clearTimeout(timeout);
    }
  }
  dispose() {
    this.stopped = true;
    this.checks.forEach((c) => c.abort());
    this.queued.clear();
    this.alerts.clear();
    this.listeners.clear();
  }
  reset(id: string) {
    this.checks.get(id)?.abort();
    this.queued.delete(id);
    this.activities.delete(id);
    this.restored.delete(id);
    this.publish();
  }
  clear() {
    this.dispose();
    try {
      this.storage().removeItem(this.storageKey);
    } catch {
      /* unavailable */
    }
  }
}
