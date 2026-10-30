import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import { afterEach, expect, it, vi } from "vitest";
import { Code, ConnectError } from "@connectrpc/connect";
import { SessionSchema } from "#gen/cxz/session_pb";
import { ProjectSchema } from "#gen/cxz/project_pb";
import {
  SessionNotificationSchema,
  SessionReadStateSchema,
  type SessionReadState,
} from "#gen/cxz/session_svc_pb";
import {
  NotificationListenResponseSchema,
  NotificationReadResponseSchema,
  NotificationReadItemSchema,
  NotificationSchema,
  NotificationReceiptSchema,
  type NotificationReadItem,
} from "#gen/cxz/payday/notification_pb";
import { SessionAttention } from "./session-attention";
const trackers: SessionAttention[] = [];
afterEach(() => trackers.splice(0).forEach((t) => t.dispose()));
function fixture() {
  const rows: NotificationReadItem[] = [],
    listeners = new Set<() => void>();
  let read = 0n;
  const readerId = new Uint8Array([16]);
  const sessions = [
    create(SessionSchema, {
      runtimeId: "one",
      project: create(ProjectSchema, { id: new Uint8Array([7]) }),
    }),
  ];
  const invalidate = () => listeners.forEach((fn) => fn());
  const api = {
    readState: vi.fn(async () =>
      create(SessionReadStateSchema, {
        readerId,
        generation: "journal",
        throughSeq: read,
      }),
    ),
    markRead: vi.fn(async (r: { throughSeq?: bigint }) => {
      read = r.throughSeq! > read ? r.throughSeq! : read;
      for (const row of rows)
        if (
          fromBinary(
            SessionNotificationSchema,
            row.notification!.payload!.value,
          ).seq <= read
        )
          row.receipt ??= create(NotificationReceiptSchema, {
            id: new Uint8Array([15]),
          });
      invalidate();
      return create(SessionReadStateSchema, {
        readerId,
        generation: "journal",
        throughSeq: read,
      });
    }),
  };
  const notifications = {
    read: vi.fn(async (r: { filter?: { unreadOnly?: boolean } }) =>
      create(NotificationReadResponseSchema, {
        readerId,
        items: rows
          .filter((row) => !r.filter?.unreadOnly || !row.receipt)
          .slice()
          .reverse(),
      }),
    ),
    listen: vi.fn(async function* (
      _r: unknown,
      options?: { signal?: AbortSignal },
    ) {
      const signal = options?.signal;
      let dirty = false,
        wake: (() => void) | undefined;
      const update = () => {
        dirty = true;
        wake?.();
      };
      listeners.add(update);
      try {
        yield create(NotificationListenResponseSchema, {
          event: { case: "ready", value: { readerId } },
        });
        while (!signal?.aborted) {
          if (!dirty)
            await new Promise<void>((resolve) => {
              wake = resolve;
              signal?.addEventListener("abort", resolve as () => void, {
                once: true,
              });
            });
          if (signal?.aborted) return;
          dirty = false;
          yield create(NotificationListenResponseSchema, {
            event: { case: "invalidated", value: {} },
          });
        }
      } finally {
        listeners.delete(update);
      }
    }),
  };
  function tracker() {
    const t = new SessionAttention(api, notifications);
    trackers.push(t);
    t.observe(sessions);
    return t;
  }
  function add(
    seq: number,
    kind = "session.completed",
    seen = false,
    generation = "journal",
  ) {
    rows.push(
      create(NotificationReadItemSchema, {
        notification: create(NotificationSchema, {
          id: new Uint8Array([seq]),
          kind,
          payload: {
            typeUrl: "type.googleapis.com/cxz.SessionNotification",
            value: toBinary(
              SessionNotificationSchema,
              create(SessionNotificationSchema, {
                runtimeId: "one",
                generation,
                seq: BigInt(seq),
              }),
            ),
          },
        }),
        receipt: seen
          ? create(NotificationReceiptSchema, { id: new Uint8Array([15]) })
          : undefined,
      }),
    );
  }
  const settled = () =>
    vi.waitFor(() =>
      expect(
        notifications.read.mock.calls.filter(([r]) => !r.filter?.unreadOnly)
          .length,
      ).toBeGreaterThan(0),
    );
  return { tracker, add, invalidate, settled, api, notifications, sessions };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

it("clears badges synchronously before ReadState or MarkRead and preserves new unread arrivals", async () => {
  const f = fixture();
  f.add(9, "session.failed");
  const state = deferred<SessionReadState>();
  const marked = deferred<void>();
  f.api.readState.mockImplementationOnce(() => state.promise);
  const mark = f.api.markRead.getMockImplementation()!;
  f.api.markRead.mockImplementationOnce(async (request) => {
    await marked.promise;
    return mark(request);
  });
  const first = f.tracker(),
    second = f.tracker();
  await vi.waitFor(() =>
    expect(first.snapshot().has("one") && second.snapshot().has("one")).toBe(
      true,
    ),
  );
  first.setReading("one", 9n);
  expect(first.snapshot().size).toBe(0);
  expect(first.errorSnapshot().size).toBe(0);
  expect(f.api.readState).not.toHaveBeenCalled();
  first.setReading("");
  expect(first.snapshot().size).toBe(0);
  expect(second.snapshot().has("one")).toBe(true);
  f.add(12);
  f.invalidate();
  await vi.waitFor(() => expect(first.snapshot().has("one")).toBe(true));
  expect(first.errorSnapshot().size).toBe(0);
  state.resolve(
    create(SessionReadStateSchema, {
      readerId: new Uint8Array([16]),
      generation: "journal",
    }),
  );
  await vi.waitFor(() => expect(f.api.markRead).toHaveBeenCalledTimes(1));
  expect(first.snapshot().has("one")).toBe(true);
  marked.resolve();
  await vi.waitFor(() => expect(second.errorSnapshot().size).toBe(0));
  expect(first.snapshot().has("one")).toBe(true);
});

it("keeps optimistic reads while a transient failure is retried", async () => {
  const f = fixture();
  f.add(4);
  f.api.markRead.mockRejectedValueOnce(
    new ConnectError("offline", Code.Unavailable),
  );
  const tracker = f.tracker();
  await vi.waitFor(() => expect(tracker.snapshot().has("one")).toBe(true));
  tracker.setReading("one", 4n);
  expect(tracker.snapshot().size).toBe(0);
  tracker.setReading("");
  await vi.waitFor(() => expect(f.api.markRead).toHaveBeenCalledTimes(1));
  expect(tracker.snapshot().size).toBe(0);
  await vi.waitFor(() => expect(f.api.markRead).toHaveBeenCalledTimes(2), {
    timeout: 3000,
  });
  expect(tracker.snapshot().size).toBe(0);
});

it("rolls back a rejected read, avoids automatic retry loops and allows reading again", async () => {
  const f = fixture();
  f.add(4);
  f.api.markRead.mockRejectedValueOnce(
    new ConnectError("read denied", Code.PermissionDenied),
  );
  const tracker = f.tracker();
  await vi.waitFor(() => expect(tracker.snapshot().has("one")).toBe(true));
  tracker.setReading("one", 4n);
  expect(tracker.snapshot().size).toBe(0);
  await vi.waitFor(() => expect(tracker.snapshot().has("one")).toBe(true));
  const reads = f.notifications.read.mock.calls.length;
  f.invalidate();
  await vi.waitFor(() =>
    expect(f.notifications.read.mock.calls.length).toBeGreaterThan(reads),
  );
  tracker.setReading("one", 4n);
  expect(tracker.snapshot().has("one")).toBe(true);
  expect(f.api.markRead).toHaveBeenCalledTimes(1);
  tracker.setReading("");
  tracker.setReading("one", 4n);
  expect(tracker.snapshot().size).toBe(0);
  await vi.waitFor(() => expect(f.api.markRead).toHaveBeenCalledTimes(2), {
    timeout: 3000,
  });
});

it("does not hide lower sequence notifications from another conversation generation", async () => {
  const f = fixture();
  f.add(9);
  const state = deferred<SessionReadState>();
  f.api.readState.mockImplementationOnce(() => state.promise);
  const tracker = f.tracker();
  await vi.waitFor(() => expect(tracker.snapshot().has("one")).toBe(true));
  tracker.setReading("one", 9n);
  tracker.setReading("");
  expect(tracker.snapshot().size).toBe(0);
  f.add(3, "session.completed", false, "replacement-journal");
  f.invalidate();
  await vi.waitFor(() => expect(tracker.snapshot().has("one")).toBe(true));
  tracker.dispose();
  state.resolve(create(SessionReadStateSchema, { generation: "journal" }));
});
it("restores server unread state silently across clients and reloads", async () => {
  const f = fixture();
  f.add(3);
  const t = f.tracker(),
    alert = vi.fn();
  t.onAlert(alert);
  await vi.waitFor(() => expect([...t.snapshot()]).toEqual(["one"]));
  expect(alert).not.toHaveBeenCalled();
  const second = f.tracker();
  await vi.waitFor(() => expect([...second.snapshot()]).toEqual(["one"]));
});
it("sounds once for each live completion, error and question", async () => {
  const f = fixture(),
    t = f.tracker(),
    alert = vi.fn();
  t.onAlert(alert);
  await f.settled();
  for (const [seq, kind, sound] of [
    [1, "session.completed", "complete"],
    [2, "session.failed", "error"],
    [3, "session.question", "question"],
  ] as const) {
    f.add(seq, kind);
    f.invalidate();
    await vi.waitFor(() => expect(alert).toHaveBeenCalledWith(sound));
  }
  f.invalidate();
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(alert).toHaveBeenCalledTimes(3);
  expect(t.errorSnapshot().has("one")).toBe(true);
});
it("persists only the visible range and shares receipts with another window", async () => {
  const f = fixture();
  f.add(9);
  const first = f.tracker(),
    second = f.tracker();
  await vi.waitFor(() => expect(second.snapshot().has("one")).toBe(true));
  first.setReading("one", 8n);
  await vi.waitFor(() => expect(f.api.markRead).toHaveBeenCalled());
  expect(second.snapshot().has("one")).toBe(true);
  first.setReading("one", 9n);
  await vi.waitFor(() => expect(second.snapshot().has("one")).toBe(false), {
    timeout: 3000,
  });
  expect(f.api.markRead.mock.calls.at(-1)?.[0].throughSeq).toBe(9n);
  const reloaded = f.tracker();
  await vi.waitFor(() =>
    expect(f.notifications.listen).toHaveBeenCalledTimes(3),
  );
  expect(reloaded.snapshot().size).toBe(0);
});
it("sounds for an already-read live completion without leaving a badge", async () => {
  const f = fixture(),
    t = f.tracker(),
    alert = vi.fn();
  t.onAlert(alert);
  await f.settled();
  f.add(4, "session.completed", true);
  f.invalidate();
  await vi.waitFor(() => expect(alert).toHaveBeenCalledWith("complete"));
  expect(t.snapshot().size).toBe(0);
});
it("does not acknowledge background sessions merely because they are listed", async () => {
  const f = fixture(),
    t = f.tracker();
  await f.settled();
  f.add(4);
  f.invalidate();
  await vi.waitFor(() => expect(t.snapshot().size).toBe(1));
  expect(f.api.markRead).not.toHaveBeenCalled();
  expect(f.api.readState).not.toHaveBeenCalled();
});
it("keeps one subscription for stable project topics and aborts it on dispose", async () => {
  const f = fixture(),
    t = f.tracker();
  await f.settled();
  t.observe(f.sessions);
  t.observe(f.sessions);
  expect(f.notifications.listen).toHaveBeenCalledTimes(1);
  const signal = f.notifications.listen.mock.calls[0][1]!.signal!;
  t.dispose();
  expect(signal.aborted).toBe(true);
});

it("does not keep retrying notification RPCs against an older Manager", async () => {
  const readState = vi.fn(),
    markRead = vi.fn(),
    read = vi.fn();
  const listen = vi.fn(async function* () {
    throw new ConnectError("old server", Code.Unimplemented);
  });
  const tracker = new SessionAttention(
    { readState, markRead },
    { read, listen },
  );
  trackers.push(tracker);
  const sessions = [
    create(SessionSchema, {
      runtimeId: "one",
      project: { id: new Uint8Array([7]) },
    }),
  ];
  tracker.observe(sessions);
  await vi.waitFor(() => expect(listen).toHaveBeenCalledTimes(1));
  await new Promise((resolve) => setTimeout(resolve, 0));
  tracker.observe(sessions);
  tracker.setReading("one", 3n);
  expect(listen).toHaveBeenCalledTimes(1);
  expect(readState).not.toHaveBeenCalled();
  expect(read).not.toHaveBeenCalled();
});
