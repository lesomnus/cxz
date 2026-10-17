import { useSyncExternalStore } from "react";
import { Code, ConnectError, type Client } from "@connectrpc/connect";
import { key, type Store } from "@lesomnus/payday/store";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { ProjectService } from "#gen/cxz/project_svc_pb";
import type { SessionService } from "#gen/cxz/session_svc_pb";
import { readBy, release } from "./stream-deadline";

/**
 * How long the Watch snapshot may take to start. The List before it has
 * already answered, so the server is known to be responsive; this is about a
 * subscription that was accepted and then never delivered.
 */
export const SNAPSHOT_DEADLINE_MS = 10_000;

type Kind = "projects" | "sessions";
// What this reads from a Watch frame, which is all two entities have in common
// at the type level: a row named by id that either carries its value or says it
// is gone. `action` is the RPC behind the change and nothing here shows it.
type WatchFrame = {
  items: readonly { id: Uint8Array; value?: Project | Session }[];
};
type Snapshot = {
  projects: Project[];
  sessions: Session[];
  loading: Record<Kind, boolean>;
  errors: Partial<Record<Kind, unknown>>;
};
const empty = (): Snapshot => ({
  projects: [],
  sessions: [],
  loading: { projects: true, sessions: true },
  errors: {},
});

// One catalog subscription per entity type and authenticated connection. Groups
// are views of this inventory, not separate server queries/subscriptions.
export class ResourceInventory {
  private state = empty();
  private listeners = new Set<() => void>();
  private stop: AbortController | undefined;

  constructor(
    private projects: Pick<Client<typeof ProjectService>, "list" | "watch">,
    private sessions: Pick<Client<typeof SessionService>, "list" | "watch">,
    private store: Pick<Store, "apply">,
  ) {}

  snapshot = () => this.state;
  refresh = () => {
    if (!this.listeners.size) return;
    this.stop?.abort();
    const stop = new AbortController();
    this.stop = stop;
    void this.watch("projects", this.projects, "cxz.Project", stop.signal);
    void this.watch("sessions", this.sessions, "cxz.Session", stop.signal);
  };
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    if (!this.stop) {
      const stop = new AbortController();
      this.stop = stop;
      void this.watch("projects", this.projects, "cxz.Project", stop.signal);
      void this.watch("sessions", this.sessions, "cxz.Session", stop.signal);
    }
    return () => {
      this.listeners.delete(listener);
      if (!this.listeners.size) {
        this.stop?.abort();
        this.stop = undefined;
      }
    };
  };

  private publish(
    kind: Kind,
    rows: Map<string, Project | Session>,
    loading: boolean,
    error?: unknown,
  ) {
    this.state = {
      ...this.state,
      [kind]: [...rows.values()],
      loading: { ...this.state.loading, [kind]: loading },
      errors: { ...this.state.errors, [kind]: error },
    };
    for (const listener of this.listeners) listener();
  }

  private async watch(
    kind: Kind,
    client:
      | Pick<Client<typeof ProjectService>, "list" | "watch">
      | Pick<Client<typeof SessionService>, "list" | "watch">,
    entity: string,
    signal: AbortSignal,
  ) {
    let retry = 0;
    const stalled = `The ${
      kind === "sessions" ? "session" : "project"
    } list stopped loading.`;
    while (!signal.aborted) {
      const rows = new Map<string, Project | Session>();
      let after = "";
      try {
        // A bounded List also settles an empty catalog: older Watch servers do
        // not emit an initial frame when no rows match. The Watch's snapshot
        // streams ALL pages, so large catalogs need no per-project paging.
        // This filter spans every project; no per-project refs are requested.
        const request = { filters: [{ listed: true }] };
        const page = await client.list({ ...request, size: 1 }, { signal });
        if (signal.aborted) return;
        after = page.next;
        for (const row of page.items) rows.set(key(row.id), row);
        this.store.apply(
          entity,
          page.items.map((value) => ({ id: value.id, value })),
        );
        // What this one row settles is only whether the catalog is empty, or
        // holds exactly one. Beyond that it is a probe, not an inventory, and
        // publishing it as a settled catalog is what made cxz#189 look
        // plausible instead of broken: one session listed, no spinner, no
        // error, forever. The snapshot owes the rest, so say it is still coming.
        const partial = after !== "";
        this.publish(kind, rows, partial);
        const stream: AsyncIterator<WatchFrame> = client
          .watch(request, {
            signal,
          })
          [Symbol.asyncIterator]();
        try {
          let first = true;
          for (;;) {
            // Only the first frame is waited on. A snapshot that has arrived
            // and a catalog that is not changing are both legitimately silent,
            // but a snapshot that never starts is not -- and nothing else here
            // can tell it from a stream that is merely quiet.
            const step = await (first && partial
              ? readBy(stream.next(), SNAPSHOT_DEADLINE_MS, stalled)
              : stream.next());
            if (step.done) break;
            if (signal.aborted) return;
            const batch = step.value;
            // A reconnect begins with a fresh snapshot, replacing stale
            // membership rather than keeping rows that disappeared while
            // disconnected.
            if (first) rows.clear();
            first = false;
            retry = 0;
            this.store.apply(entity, batch.items);
            for (const item of batch.items) {
              const id = key(item.id);
              if (item.value) rows.set(id, item.value);
              else rows.delete(id);
            }
            this.publish(kind, rows, false);
          }
        } finally {
          release(stream);
        }
        throw new Error("Resource subscription disconnected");
      } catch (error) {
        if (signal.aborted) return;
        const code = ConnectError.from(error).code;
        if (code === Code.InvalidArgument || code === Code.Unimplemented) {
          // Older managers only allow watches naming individual resource IDs.
          // Keep their full paginated inventory usable, without repeatedly
          // reopening a subscription this server cannot support.
          try {
            while (after) {
              const page = await client.list(
                { filters: [{ listed: true }], size: 200, after },
                { signal },
              );
              if (signal.aborted) return;
              after = page.next;
              for (const row of page.items) rows.set(key(row.id), row);
              this.store.apply(
                entity,
                page.items.map((value) => ({ id: value.id, value })),
              );
            }
            error = new Error(
              "Update the cxz Manager to enable shared resource subscriptions.",
            );
          } catch (failure) {
            error = failure;
          }
          if (!signal.aborted) this.publish(kind, rows, false, error);
          return;
        }
        this.publish(kind, rows, false, error);
      }
      if (signal.aborted) return;
      // Reconnect without multiplying streams or spinning on a rejected RPC.
      const delay = Math.min(30000, 500 * 2 ** Math.min(retry++, 6));
      await new Promise<void>((resolve) => {
        const done = () => {
          clearTimeout(timer);
          signal.removeEventListener("abort", done);
          resolve();
        };
        const timer = setTimeout(done, delay);
        signal.addEventListener("abort", done, { once: true });
      });
    }
  }
}

export function useResourceInventory(connection: {
  inventory: ResourceInventory;
}) {
  return useSyncExternalStore(
    connection.inventory.subscribe,
    connection.inventory.snapshot,
  );
}
