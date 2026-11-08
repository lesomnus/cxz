import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { expect, it, vi } from "vitest";
import { SessionSchema, SessionEventSchema } from "#gen/cxz/session_pb";
import { key } from "@lesomnus/payday/store";
import {
  activityTime,
  activityVersion,
  loadSessionActivity,
  type ActivityCache,
} from "#src/features/session/model/session-activity-time.ts";
import { projectActivitySessions } from "./project-session-activity";

const session = (id: number, state = "idle", project = 1) =>
  create(SessionSchema, {
    runtimeId: `session-${id}`,
    project: { id: new Uint8Array([project]) },
    status: { state, runId: "run", lastSeq: 5n },
  });
const event = (time: number) =>
  create(SessionEventSchema, { timeMs: BigInt(time) });

it("prioritizes unread completed turns, then working sessions, then actual conversation time within each project", () => {
  const recent = session(1),
    working = session(2, "working"),
    unread = session(3),
    other = session(4, "idle", 2);
  const activity: ActivityCache = new Map([
    [recent.runtimeId, { version: "", time: 300 }],
    [working.runtimeId, { version: "", time: 200 }],
    [unread.runtimeId, { version: "", time: 100 }],
  ]);
  const sessions = [recent, working, unread, other],
    project = key(recent.project!.id);
  expect(
    projectActivitySessions(
      sessions,
      new Set([unread.runtimeId]),
      activity,
    ).get(project),
  ).toBe(unread);
  expect(
    projectActivitySessions(sessions, new Set(), activity).get(project),
  ).toBe(working);
  working.status!.state = "idle";
  unread.dateUpdated = timestampFromDate(new Date());
  expect(
    projectActivitySessions(sessions, new Set(), activity).get(project),
  ).toBe(recent);
  expect(
    projectActivitySessions(sessions, new Set(), activity).get(
      key(other.project!.id),
    ),
  ).toBe(other);
  recent.dateErased = timestampFromDate(new Date());
  expect(
    projectActivitySessions(sessions, new Set(), activity).get(project),
  ).toBe(working);
});

it("uses the latest timestamp in a projected page including its preceding input", () => {
  expect(activityTime([event(100), event(80)], event(120))).toBe(120);
  expect(activityTime([event(0)])).toBeUndefined();
});

it("bounds concurrent one-row reads, caches unchanged inventory, and refetches changed sequences", async () => {
  const sessions = Array.from({ length: 8 }, (_, i) => session(i)),
    cache: ActivityCache = new Map();
  let inFlight = 0,
    peak = 0;
  const read = vi.fn(async () => {
    peak = Math.max(peak, ++inFlight);
    await new Promise((resolve) => setTimeout(resolve, 1));
    inFlight--;
    return { events: [event(100)] };
  });
  const changed = vi.fn(),
    signal = new AbortController().signal;
  await loadSessionActivity(sessions, cache, signal, read, changed);
  expect(peak).toBe(3);
  expect(cache.size).toBe(8);
  await loadSessionActivity(sessions, cache, signal, read, changed);
  expect(read).toHaveBeenCalledTimes(8);
  sessions[0]!.status!.lastSeq++;
  await loadSessionActivity(sessions, cache, signal, read, changed);
  expect(read).toHaveBeenCalledTimes(9);
  expect(cache.get(sessions[0]!.runtimeId)?.version).toBe(
    activityVersion(sessions[0]!),
  );
});

it("retains successful data on failures and ignores replies after leaving the page", async () => {
  const s = session(1),
    cache: ActivityCache = new Map([
      [s.runtimeId, { version: "older", time: 100 }],
    ]);
  const changed = vi.fn(),
    controller = new AbortController();
  await loadSessionActivity(
    [s],
    cache,
    controller.signal,
    async () => {
      throw new Error("offline");
    },
    changed,
  );
  expect(cache.get(s.runtimeId)?.time).toBe(100);
  await loadSessionActivity(
    [s],
    cache,
    controller.signal,
    async () => {
      controller.abort();
      return { events: [event(200)] };
    },
    changed,
  );
  expect(cache.get(s.runtimeId)?.time).toBe(100);
  expect(changed).not.toHaveBeenCalled();
});
