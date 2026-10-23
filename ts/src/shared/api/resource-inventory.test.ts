import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { SessionSchema, SessionStatusSchema } from "#gen/cxz/session_pb";
import {
  ProjectListResponseSchema,
  ProjectWatchResponseSchema,
} from "#gen/cxz/project_svc_pb";
import {
  SessionListResponseSchema,
  SessionWatchResponseSchema,
} from "#gen/cxz/session_svc_pb";
import { ResourceInventory, SNAPSHOT_DEADLINE_MS } from "./resource-inventory";

class Feed<T> {
  private values: T[] = [];
  private waiting: ((value: T | undefined) => void) | undefined;
  private ended = false;
  push(value: T) {
    if (this.waiting) {
      const next = this.waiting;
      this.waiting = undefined;
      next(value);
    } else this.values.push(value);
  }
  end() {
    this.ended = true;
    this.waiting?.(undefined);
  }
  async *read(signal?: AbortSignal) {
    while (!signal?.aborted && !this.ended) {
      const next =
        this.values.shift() ??
        (await new Promise<T | undefined>((resolve) => {
          const finish = (value: T | undefined) => {
            signal?.removeEventListener("abort", abort);
            if (this.waiting === finish) this.waiting = undefined;
            resolve(value);
          };
          const abort = () => finish(undefined);
          signal?.addEventListener("abort", abort, { once: true });
          this.waiting = finish;
        }));
      if (next === undefined || signal?.aborted) return;
      yield next;
    }
  }
}
const project = (i: number) =>
  create(ProjectSchema, {
    id: new TextEncoder().encode(`project-${String(i).padStart(8, "0")}`),
    runtimeId: `project-${i}`,
  });
const session = (i: number, p = project(i)) =>
  create(SessionSchema, {
    id: new TextEncoder().encode(`session-${String(i).padStart(8, "0")}`),
    runtimeId: `session-${i}`,
    project: p,
    listed: true,
  });
function fixture(empty = false) {
  const p = new Feed<
    ReturnType<typeof create<typeof ProjectWatchResponseSchema>>
  >();
  const s = new Feed<
    ReturnType<typeof create<typeof SessionWatchResponseSchema>>
  >();
  const projects = {
    list: vi.fn(async () =>
      create(ProjectListResponseSchema, { items: empty ? [] : [project(0)] }),
    ),
    watch: vi.fn((_request, options) => p.read(options?.signal)),
  };
  const sessions = {
    list: vi.fn(async () =>
      create(SessionListResponseSchema, { items: empty ? [] : [session(0)] }),
    ),
    watch: vi.fn((_request, options) => s.read(options?.signal)),
  };
  const store = { apply: vi.fn() };
  const inventory = new ResourceInventory(projects, sessions, store);
  return { inventory, projects, sessions, p, s, store };
}
afterEach(() => vi.useRealTimers());

describe("shared resource inventory", () => {
  it("explicit refresh replaces the shared streams and stops when no view subscribes", async () => {
    vi.useFakeTimers();
    const f = fixture();
    const off = f.inventory.subscribe(() => {});
    await vi.advanceTimersByTimeAsync(0);
    const previous = f.sessions.watch.mock.calls[0][1].signal;
    f.inventory.refresh();
    await vi.advanceTimersByTimeAsync(0);
    expect(previous.aborted).toBe(true);
    expect(f.sessions.list).toHaveBeenCalledTimes(2);
    expect(f.sessions.watch).toHaveBeenCalledTimes(2);
    expect(f.projects.watch).toHaveBeenCalledTimes(2);
    off();
    f.inventory.refresh();
    await vi.advanceTimersByTimeAsync(0);
    expect(f.sessions.watch).toHaveBeenCalledTimes(2);
  });
  it("reads all fallback pages once when an older manager rejects global watches", async () => {
    vi.useFakeTimers();
    const f = fixture();
    f.projects.list.mockResolvedValueOnce(
      create(ProjectListResponseSchema, {
        items: [project(0)],
        next: "cursor",
      }),
    );
    f.projects.list.mockResolvedValue(
      create(ProjectListResponseSchema, { items: [project(1), project(2)] }),
    );
    f.projects.watch.mockImplementation(() => {
      throw new ConnectError("individual refs required", Code.InvalidArgument);
    });
    const off = f.inventory.subscribe(() => {});
    try {
      await vi.advanceTimersByTimeAsync(30000);
      expect(f.inventory.snapshot().projects.map((p) => p.runtimeId)).toEqual([
        "project-0",
        "project-1",
        "project-2",
      ]);
      expect(String(f.inventory.snapshot().errors.projects)).toContain(
        "Update the cxz Manager",
      );
      expect(f.projects.list).toHaveBeenCalledTimes(2);
      expect(f.projects.watch).toHaveBeenCalledTimes(1);
    } finally {
      off();
    }
  });
  it("uses one list/watch pair per type for many projects, handles chunked snapshots, moves, updates and removals", async () => {
    const f = fixture();
    const off = f.inventory.subscribe(() => {});
    const second = f.inventory.subscribe(() => {});
    try {
      await vi.waitFor(() => expect(f.sessions.watch).toHaveBeenCalledTimes(1));
      const ps = Array.from({ length: 80 }, (_, i) => project(i));
      const ss = ps.map((p, i) => session(i, p));
      for (let i = 0; i < ps.length; i += 20) {
        f.p.push(
          create(ProjectWatchResponseSchema, {
            snapshotComplete: i + 20 >= ps.length,
            items: ps
              .slice(i, i + 20)
              .map((value) => ({ id: value.id, value })),
          }),
        );
        f.s.push(
          create(SessionWatchResponseSchema, {
            snapshotComplete: i + 20 >= ss.length,
            items: ss
              .slice(i, i + 20)
              .map((value) => ({ id: value.id, value })),
          }),
        );
      }
      await vi.waitFor(() =>
        expect(f.inventory.snapshot().sessions).toHaveLength(80),
      );
      expect(f.inventory.snapshot().projects).toHaveLength(80);
      expect(f.projects.list).toHaveBeenCalledTimes(1);
      expect(f.projects.watch).toHaveBeenCalledTimes(1);
      expect(f.sessions.list).toHaveBeenCalledTimes(1);
      expect(f.sessions.watch).toHaveBeenCalledWith(
        { filters: [{ listed: true }] },
        expect.anything(),
      );
      // Moving to another project and adding/removing rows changes membership
      // immediately, without revalidating a list for each affected group.
      const moved = create(SessionSchema, {
        ...ss[0],
        project: ps[1],
        name: "Changed",
        status: create(SessionStatusSchema, { state: "running" }),
      });
      f.s.push(
        create(SessionWatchResponseSchema, {
          items: [
            { id: moved.id, value: moved },
            { id: ss[2].id },
            { id: session(90, ps[1]).id, value: session(90, ps[1]) },
          ],
        }),
      );
      await vi.waitFor(() =>
        expect(f.inventory.snapshot().sessions[0].name).toBe("Changed"),
      );
      expect(f.inventory.snapshot().sessions[0].project?.runtimeId).toBe(
        "project-1",
      );
      expect(
        f.inventory
          .snapshot()
          .sessions.some((s) => s.runtimeId === "session-2"),
      ).toBe(false);
      expect(
        f.inventory
          .snapshot()
          .sessions.some((s) => s.runtimeId === "session-90"),
      ).toBe(true);
      expect(f.store.apply).toHaveBeenCalledWith(
        "cxz.Session",
        expect.arrayContaining([expect.objectContaining({ id: ss[2].id })]),
      );
      off();
      expect(f.sessions.watch.mock.calls[0][1]?.signal.aborted).toBe(false);
    } finally {
      off();
      second();
    }
    expect(f.projects.watch.mock.calls[0][1]?.signal.aborted).toBe(true);
    expect(f.sessions.watch.mock.calls[0][1]?.signal.aborted).toBe(true);
  });

  it("settles empty catalogs even when Watch sends no snapshot frame and still receives future additions", async () => {
    const f = fixture(true);
    const off = f.inventory.subscribe(() => {});
    try {
      await vi.waitFor(() =>
        expect(f.inventory.snapshot().loading).toEqual({
          projects: false,
          sessions: false,
        }),
      );
      expect(f.inventory.snapshot().sessions).toEqual([]);
      f.p.push(
        create(ProjectWatchResponseSchema, {
          items: [{ id: project(1).id, value: project(1) }],
        }),
      );
      f.s.push(
        create(SessionWatchResponseSchema, {
          items: [{ id: session(1).id, value: session(1) }],
        }),
      );
      await vi.waitFor(() =>
        expect(f.inventory.snapshot().sessions).toHaveLength(1),
      );
      expect(f.inventory.snapshot().projects).toHaveLength(1);
    } finally {
      off();
    }
  });

  // cxz#189: the bounded List is a probe for whether the catalog is empty, not
  // an inventory. Published as a settled one it produced a session list with a
  // single row, no spinner, no error and no retry -- for as long as the page
  // stayed open, because a Watch that never sends its snapshot neither ends nor
  // throws.
  it("does not let one listed row stand in for a catalog whose snapshot never arrives", async () => {
    vi.useFakeTimers();
    const f = fixture();
    f.sessions.list.mockResolvedValue(
      create(SessionListResponseSchema, { items: [session(0)], next: "more" }),
    );
    const off = f.inventory.subscribe(() => {});
    try {
      await vi.advanceTimersByTimeAsync(0);
      expect(f.inventory.snapshot().sessions.map((s) => s.runtimeId)).toEqual([
        "session-0",
      ]);
      // Shown, because it is a row the server sent, and still loading, because
      // the list said there was more and the snapshot owes it.
      expect(f.inventory.snapshot().loading.sessions).toBe(true);
      await vi.advanceTimersByTimeAsync(SNAPSHOT_DEADLINE_MS - 1);
      expect(f.inventory.snapshot().errors.sessions).toBeUndefined();
      await vi.advanceTimersByTimeAsync(1);
      expect(String(f.inventory.snapshot().errors.sessions)).toContain(
        "stopped loading",
      );
      expect(f.sessions.watch).toHaveBeenCalledTimes(1);
      expect(f.sessions.watch.mock.calls[0][1].signal.aborted).toBe(true);
      await vi.advanceTimersByTimeAsync(500);
      expect(f.sessions.watch).toHaveBeenCalledTimes(2);
      f.s.push(
        create(SessionWatchResponseSchema, {
          items: [session(0), session(1)].map((value) => ({
            id: value.id,
            value,
          })),
        }),
      );
      await vi.advanceTimersByTimeAsync(0);
      expect(f.inventory.snapshot().loading.sessions).toBe(false);
      expect(f.inventory.snapshot().sessions).toHaveLength(2);
      expect(f.inventory.snapshot().errors.sessions).toBeUndefined();
    } finally {
      off();
    }
  });
  // The other half of the same probe: a catalog the List already answered in
  // full owes nothing, and an old server sends no snapshot frame for one. That
  // silence is the expected answer, so waiting on it would report a stall on
  // every empty account.
  it("never reports a stall on a catalog the list already settled", async () => {
    vi.useFakeTimers();
    const f = fixture(true);
    const off = f.inventory.subscribe(() => {});
    try {
      await vi.advanceTimersByTimeAsync(SNAPSHOT_DEADLINE_MS * 2);
      expect(f.inventory.snapshot().loading).toEqual({
        projects: false,
        sessions: false,
      });
      expect(f.inventory.snapshot().errors).toEqual({});
      expect(f.sessions.watch).toHaveBeenCalledTimes(1);
      expect(f.projects.watch).toHaveBeenCalledTimes(1);
    } finally {
      off();
    }
  });

  it("backs off a disconnected stream, replaces stale inventory on reconnect and cancels pending retries", async () => {
    vi.useFakeTimers();
    const f = fixture();
    const off = f.inventory.subscribe(() => {});
    await vi.advanceTimersByTimeAsync(0);
    const next = new Feed<
      ReturnType<typeof create<typeof SessionWatchResponseSchema>>
    >();
    f.sessions.list.mockResolvedValue(
      create(SessionListResponseSchema, { items: [] }),
    );
    f.sessions.watch.mockImplementationOnce((_request, options) =>
      next.read(options?.signal),
    );
    f.s.end();
    await vi.advanceTimersByTimeAsync(499);
    expect(f.sessions.watch).toHaveBeenCalledTimes(1);
    expect(f.inventory.snapshot().errors.sessions).toBeUndefined();
    await vi.advanceTimersByTimeAsync(1);
    expect(f.sessions.watch).toHaveBeenCalledTimes(2);
    expect(f.inventory.snapshot().sessions).toEqual([]);
    next.push(
      create(SessionWatchResponseSchema, {
        items: [{ id: session(2).id, value: session(2) }],
      }),
    );
    await vi.advanceTimersByTimeAsync(0);
    expect(f.inventory.snapshot().sessions.map((s) => s.runtimeId)).toEqual([
      "session-2",
    ]);
    next.end();
    await vi.advanceTimersByTimeAsync(0);
    off();
    await vi.advanceTimersByTimeAsync(30000);
    expect(f.sessions.watch).toHaveBeenCalledTimes(2);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("keeps unchanged rows, arrays and store subscribers untouched by replayed updates", async () => {
    vi.useFakeTimers();
    const f = fixture();
    const changed = vi.fn();
    const off = f.inventory.subscribe(changed);
    try {
      await vi.advanceTimersByTimeAsync(0);
      const ps = [project(0), project(1)];
      const ss = ps.map((p, i) => session(i, p));
      f.p.push(
        create(ProjectWatchResponseSchema, {
          snapshotComplete: true,
          items: ps.map((value) => ({ id: value.id, value })),
        }),
      );
      f.s.push(
        create(SessionWatchResponseSchema, {
          snapshotComplete: true,
          items: ss.map((value) => ({ id: value.id, value })),
        }),
      );
      await vi.advanceTimersByTimeAsync(0);
      const before = f.inventory.snapshot();
      f.store.apply.mockClear();
      changed.mockClear();
      // The RPC decoder produces new objects even when every field is equal.
      f.p.push(
        create(ProjectWatchResponseSchema, {
          items: ps.map((p) => ({
            id: p.id,
            value: project(Number(p.runtimeId.split("-")[1])),
          })),
        }),
      );
      f.s.push(
        create(SessionWatchResponseSchema, {
          items: ss.map((s, i) => ({ id: s.id, value: session(i) })),
        }),
      );
      await vi.advanceTimersByTimeAsync(0);
      expect(f.inventory.snapshot()).toBe(before);
      expect(changed).not.toHaveBeenCalled();
      expect(f.store.apply).not.toHaveBeenCalled();

      const updated = create(SessionSchema, { ...session(1), name: "Renamed" });
      f.s.push(
        create(SessionWatchResponseSchema, {
          items: [
            { id: ss[0].id, value: session(0) },
            { id: updated.id, value: updated },
          ],
        }),
      );
      await vi.advanceTimersByTimeAsync(0);
      const after = f.inventory.snapshot();
      expect(after.projects).toBe(before.projects);
      expect(after.sessions[0]).toBe(before.sessions[0]);
      expect(after.sessions[1].name).toBe("Renamed");
      expect(changed).toHaveBeenCalledOnce();
      expect(f.store.apply).toHaveBeenCalledExactlyOnceWith("cxz.Session", [
        { id: updated.id, value: updated },
      ]);
    } finally {
      off();
    }
  });

  it("retains the complete catalog through refresh probes and incomplete snapshot pages", async () => {
    vi.useFakeTimers();
    const f = fixture();
    f.projects.list.mockResolvedValue(
      create(ProjectListResponseSchema, {
        items: [project(0)],
        next: "more",
      }),
    );
    f.sessions.list.mockResolvedValue(
      create(SessionListResponseSchema, {
        items: [session(0)],
        next: "more",
      }),
    );
    const changed = vi.fn();
    const off = f.inventory.subscribe(changed);
    const ps = Array.from({ length: 80 }, (_, i) => project(i));
    const ss = ps.map((p, i) => session(i, p));
    const send = (start: number, end: number, complete: boolean) => {
      f.p.push(
        create(ProjectWatchResponseSchema, {
          snapshotComplete: complete,
          items: ps.slice(start, end).map((value) => ({ id: value.id, value })),
        }),
      );
      f.s.push(
        create(SessionWatchResponseSchema, {
          snapshotComplete: complete,
          items: ss.slice(start, end).map((value) => ({ id: value.id, value })),
        }),
      );
    };
    try {
      await vi.advanceTimersByTimeAsync(0);
      send(0, 80, true);
      await vi.advanceTimersByTimeAsync(0);
      const before = f.inventory.snapshot();
      const signal = f.sessions.watch.mock.calls[0][1].signal;
      f.store.apply.mockClear();
      changed.mockClear();
      f.inventory.refresh();
      await vi.advanceTimersByTimeAsync(0);
      expect(signal.aborted).toBe(true);
      expect(f.inventory.snapshot()).toBe(before);
      send(0, 20, false);
      await vi.advanceTimersByTimeAsync(0);
      expect(f.inventory.snapshot()).toBe(before);
      send(20, 60, false);
      await vi.advanceTimersByTimeAsync(0);
      expect(f.inventory.snapshot()).toBe(before);
      send(60, 80, true);
      await vi.advanceTimersByTimeAsync(0);
      expect(f.inventory.snapshot()).toBe(before);
      expect(changed).not.toHaveBeenCalled();
      expect(f.store.apply).not.toHaveBeenCalled();
      expect(f.inventory.snapshot().loading).toEqual({
        projects: false,
        sessions: false,
      });
    } finally {
      off();
    }
  });

  it("commits offline removals only with a complete replacement snapshot, including the store", async () => {
    vi.useFakeTimers();
    const f = fixture();
    f.sessions.list.mockResolvedValue(
      create(SessionListResponseSchema, {
        items: [session(0)],
        next: "more",
      }),
    );
    const off = f.inventory.subscribe(() => {});
    const ss = [session(0), session(1), session(2)];
    try {
      await vi.advanceTimersByTimeAsync(0);
      f.s.push(
        create(SessionWatchResponseSchema, {
          snapshotComplete: true,
          items: ss.map((value) => ({ id: value.id, value })),
        }),
      );
      await vi.advanceTimersByTimeAsync(0);
      const before = f.inventory.snapshot().sessions;
      f.inventory.refresh();
      await vi.advanceTimersByTimeAsync(0);
      f.store.apply.mockClear();
      f.s.push(
        create(SessionWatchResponseSchema, {
          snapshotComplete: false,
          items: [{ id: ss[0].id, value: session(0) }],
        }),
      );
      await vi.advanceTimersByTimeAsync(0);
      expect(f.inventory.snapshot().sessions).toBe(before);
      f.s.push(
        create(SessionWatchResponseSchema, {
          snapshotComplete: true,
          items: [{ id: ss[2].id, value: session(2) }],
        }),
      );
      await vi.advanceTimersByTimeAsync(0);
      expect(f.inventory.snapshot().sessions).toEqual([ss[0], ss[2]]);
      expect(f.inventory.snapshot().sessions[0]).toBe(before[0]);
      expect(f.inventory.snapshot().sessions[1]).toBe(before[2]);
      expect(f.store.apply).toHaveBeenCalledExactlyOnceWith("cxz.Session", [
        { id: ss[1].id },
      ]);
    } finally {
      off();
    }
  });
});
