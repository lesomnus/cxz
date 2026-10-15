import type { Client } from "@connectrpc/connect";
import type { SessionService } from "#gen/cxz/session_svc_pb";
import type { Session } from "#gen/cxz/session_pb";
import { BackgroundJobs } from "./background-jobs";

type Request = { id: string; run: string; revision: bigint };
const same = (a: Request | undefined, b: Request) =>
  a?.run === b.run && a.revision === b.revision;

// Catalog-driven snapshots, without a live stream for every sidebar row.
export class SessionBackground {
  private desired = new Map<string, Request>();
  private loaded = new Map<string, Request>();
  private queued = new Map<string, Request>();
  private pending = new Map<
    string,
    { request: Request; controller: AbortController }
  >();
  private value = new Set<string>();
  private listeners = new Set<() => void>();
  constructor(private api: Pick<Client<typeof SessionService>, "background">) {}
  snapshot = () => this.value;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  private mark(id: string, active: boolean) {
    if (this.value.has(id) === active) return;
    this.value = new Set(this.value);
    if (active) this.value.add(id);
    else this.value.delete(id);
    this.listeners.forEach((listener) => listener());
  }
  observe(sessions: Session[]) {
    const desired = new Map<string, Request>();
    for (const session of sessions) {
      const status = session.status;
      if (
        !status?.runId ||
        status.state === "stopped" ||
        ["running", "working", "waiting_input", "waiting"].includes(
          status.state,
        )
      )
        continue;
      const request = {
        id: session.runtimeId,
        run: status.runId,
        revision: status.lastSeq,
      };
      desired.set(request.id, request);
      if (this.desired.get(request.id)?.run !== request.run)
        this.mark(request.id, false);
      if (
        !same(this.loaded.get(request.id), request) &&
        (!same(this.pending.get(request.id)?.request, request) ||
          this.pending.get(request.id)?.controller.signal.aborted)
      )
        this.queued.set(request.id, request);
    }
    this.desired = desired;
    for (const id of this.value) if (!desired.has(id)) this.mark(id, false);
    for (const id of this.loaded.keys())
      if (!desired.has(id)) this.loaded.delete(id);
    for (const id of this.queued.keys())
      if (!desired.has(id)) this.queued.delete(id);
    for (const [id, pending] of this.pending)
      if (!desired.has(id)) pending.controller.abort();
    this.pump();
  }
  private pump() {
    for (const [id, request] of this.queued) {
      if (this.pending.size >= 4) break;
      if (this.pending.has(id)) continue;
      this.queued.delete(id);
      const controller = new AbortController();
      this.pending.set(id, { request, controller });
      void this.api
        .background(
          { ref: { key: { case: "runtimeId", value: id } } },
          { signal: controller.signal, timeoutMs: 10000 },
        )
        .then((reply) => {
          if (controller.signal.aborted || !same(this.desired.get(id), request))
            return;
          const jobs = new BackgroundJobs(request.run);
          jobs.seed(reply.data, reply.lastSeq);
          this.loaded.set(id, request);
          this.mark(id, (jobs.snapshot().count ?? 0) > 0);
        })
        .catch(() => {
          if (!controller.signal.aborted && same(this.desired.get(id), request))
            this.mark(id, false);
        })
        .finally(() => {
          this.pending.delete(id);
          this.pump();
        });
    }
  }
  dispose() {
    this.queued.clear();
    this.desired.clear();
    this.loaded.clear();
    for (const pending of this.pending.values()) pending.controller.abort();
  }
}
