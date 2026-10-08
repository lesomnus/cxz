import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProjectSchema } from "../gen/cxz/project_pb";
import { SessionSchema, SessionStatusSchema } from "../gen/cxz/session_pb";
import {
  ProjectListResponseSchema,
  ProjectWatchResponseSchema,
} from "../gen/cxz/project_svc_pb";
import {
  SessionListResponseSchema,
  SessionWatchResponseSchema,
} from "../gen/cxz/session_svc_pb";
import { ResourceInventory } from "./resource-inventory";

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
          const abort = () => resolve(undefined);
          signal?.addEventListener("abort", abort, { once: true });
          this.waiting = (value) => {
            signal?.removeEventListener("abort", abort);
            resolve(value);
          };
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
    listed: true,
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
            items: ps
              .slice(i, i + 20)
              .map((value) => ({ id: value.id, value })),
          }),
        );
        f.s.push(
          create(SessionWatchResponseSchema, {
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
    expect(f.inventory.snapshot().errors.sessions).toBeDefined();
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
});
