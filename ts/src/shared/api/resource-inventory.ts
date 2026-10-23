import { useSyncExternalStore } from "react";
import { equals, isFieldSet, type Message } from "@bufbuild/protobuf";
import { Code, ConnectError, type Client } from "@connectrpc/connect";
import { key, type Store } from "@lesomnus/payday/store";
import { ProjectSchema, type Project } from "#gen/cxz/project_pb";
import { SessionSchema, type Session } from "#gen/cxz/session_pb";
import {
  ProjectWatchResponseSchema,
  type ProjectService,
} from "#gen/cxz/project_svc_pb";
import {
  SessionWatchResponseSchema,
  type SessionService,
} from "#gen/cxz/session_svc_pb";
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
type WatchFrame = Message & {
  items: readonly { id: Uint8Array; value?: Project | Session }[];
  snapshotComplete?: boolean;
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
  private settled = { projects: false, sessions: false };

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
    void this.watch("projects", this.projects, stop.signal);
    void this.watch("sessions", this.sessions, stop.signal);
  };
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    if (!this.stop) {
      const stop = new AbortController();
      this.stop = stop;
      void this.watch("projects", this.projects, stop.signal);
      void this.watch("sessions", this.sessions, stop.signal);
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
    const previous = this.state[kind];
    const byId = new Map(previous.map((row) => [key(row.id), row]));
    const changes: { id: Uint8Array; value?: Project | Session }[] = [];
    const next = [...rows.values()].map((row) => {
      const old = byId.get(key(row.id));
      byId.delete(key(row.id));
      if (
        old &&
        (kind === "projects"
          ? equals(ProjectSchema, old as Project, row as Project)
          : equals(SessionSchema, old as Session, row as Session))
      )
        return old;
      changes.push({ id: row.id, value: row });
      return row;
    });
    for (const row of byId.values()) changes.push({ id: row.id });
    const same =
      next.length === previous.length &&
      next.every((row, i) => row === previous[i]);
    if (
      same &&
      this.state.loading[kind] === loading &&
      this.state.errors[kind] === error
    )
      return;
    if (changes.length)
      this.store.apply(
        kind === "projects" ? "cxz.Project" : "cxz.Session",
        changes,
      );
    this.state = {
      ...this.state,
      [kind]: same ? previous : next,
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
    signal: AbortSignal,
  ) {
    let retry = 0;
    const stalled = `The ${
      kind === "sessions" ? "session" : "project"
    } list stopped loading.`;
    // Sessions keep an archived half, so listing them says which half it wants.
    // Projects do not: a project is registered or erased, and the default list
    // is every registered one. See cxz#274.
    const request = kind === "sessions" ? { filters: [{ listed: true }] } : {};
    const completion =
      kind === "projects"
        ? ProjectWatchResponseSchema.field.snapshotComplete
        : SessionWatchResponseSchema.field.snapshotComplete;
    while (!signal.aborted) {
      let rows = new Map<string, Project | Session>(
        this.state[kind].map((row) => [key(row.id), row]),
      );
      const hasSnapshot = this.settled[kind];
      const listed = new Map<string, Project | Session>();
      const attempt = new AbortController();
      const abort = () => attempt.abort();
      signal.addEventListener("abort", abort, { once: true });
      if (signal.aborted) abort();
      let after = "";
      try {
        // A bounded List also settles an empty catalog: older Watch servers do
        // not emit an initial frame when no rows match. The Watch's snapshot
        // streams ALL pages, so large catalogs need no per-project paging.
        const page = await client.list(
          { ...request, size: 1 },
          { signal: attempt.signal },
        );
        if (signal.aborted) return;
        after = page.next;
        for (const row of page.items) listed.set(key(row.id), row);
        // What this one row settles is only whether the catalog is empty, or
        // holds exactly one. Beyond that it is a probe, not an inventory, and
        // publishing it as a settled catalog is what made cxz#189 look
        // plausible instead of broken: one session listed, no spinner, no
        // error, forever. The snapshot owes the rest, so say it is still coming.
        const partial = after !== "";
        if (!partial) {
          rows = new Map(listed);
          this.publish(kind, rows, false);
          this.settled[kind] = true;
        } else if (!hasSnapshot) {
          for (const [id, row] of listed) rows.set(id, row);
          this.publish(kind, rows, true);
        }
        const stream: AsyncIterator<WatchFrame> = client
          .watch(request, {
            signal: attempt.signal,
          })
          [Symbol.asyncIterator]();
        try {
          let first = true;
          let pending = partial;
          const snapshot = new Map<string, Project | Session>();
          for (;;) {
            // Snapshot pages have a deadline; a settled subscription may stay
            // silent indefinitely when nothing changes.
            const step = await (first && pending
              ? readBy(stream.next(), SNAPSHOT_DEADLINE_MS, stalled)
              : stream.next());
            if (step.done) break;
            if (signal.aborted) return;
            const batch = step.value;
            retry = 0;
            const target = first ? snapshot : rows;
            for (const item of batch.items) {
              const id = key(item.id);
              if (item.value) target.set(id, item.value);
              else target.delete(id);
            }
            if (first) {
              if (isFieldSet(batch, completion) && !batch.snapshotComplete) {
                pending = true;
                if (!hasSnapshot) {
                  for (const [id, row] of snapshot) rows.set(id, row);
                  this.publish(kind, rows, true);
                }
                continue;
              }
              // Old managers send their snapshot without a completion field.
              // New managers mark its final page so a refresh never briefly
              // removes rows that are still in flight.
              rows = snapshot;
              first = false;
              this.settled[kind] = true;
            }
            this.publish(kind, rows, false);
          }
        } finally {
          release(stream);
        }
        if (!this.settled[kind])
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
                { ...request, size: 200, after },
                { signal: attempt.signal },
              );
              if (signal.aborted) return;
              after = page.next;
              for (const row of page.items) listed.set(key(row.id), row);
            }
            rows = listed;
            this.settled[kind] = true;
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
      } finally {
        attempt.abort();
        signal.removeEventListener("abort", abort);
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
