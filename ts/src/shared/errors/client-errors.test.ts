import { afterEach, expect, it, vi } from "vitest";
import { Code, ConnectError } from "@connectrpc/connect";
import { ClientErrors, errorStaleAfter, isAPIError } from "./client-errors";

afterEach(() => vi.useRealTimers());
function storage() {
  const values = new Map<string, string>();
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value);
    },
    values,
  };
}
it("deduplicates an active failure and retains a separate episode after recovery", () => {
  const errors = new ClientErrors();
  const error = new ConnectError("offline", Code.Unavailable);
  errors.report("account:A", error, { source: "rpc" });
  errors.report("account:A", error, { source: "rpc" });
  expect(errors.snapshot()).toHaveLength(1);
  expect(errors.snapshot()[0]).toMatchObject({
    status: "active",
    count: 2,
    code: "Unavailable",
  });
  errors.resolve("account:B");
  expect(errors.snapshot()[0].status).toBe("active");
  errors.resolve("account:A");
  expect(errors.snapshot()[0]).toMatchObject({ status: "resolved", count: 2 });
  errors.report("account:A", error, { source: "rpc" });
  expect(errors.snapshot().map((e) => e.status)).toEqual([
    "active",
    "resolved",
  ]);
});
it("restores redacted active/resolved history, keeps clear-history bounded and ignores cancellation", () => {
  vi.useFakeTimers();
  const disk = storage();
  const errors = new ClientErrors({ storage: disk, limit: 3 });
  const input =
    "Bearer private-credential token=private-password https://user:pass@example.com/api?secret=private-query eyJhbG.fake.signature";
  errors.report("A", new Error(input), {
    source: "rpc",
    connection: "https://user:pass@example.com/api?secret=private-query",
  });
  errors.report("cancel", new ConnectError("canceled", Code.Canceled));
  errors.report("B", new Error("second"), { source: "rpc" });
  errors.resolve("A");
  errors.flush();
  const raw = [...disk.values.values()].join("");
  for (const secret of [
    "private-credential",
    "private-password",
    "private-query",
    "user:pass",
    "eyJhbG.fake.signature",
  ])
    expect(raw).not.toContain(secret);
  const restored = new ClientErrors({ storage: disk, limit: 3 });
  expect(restored.snapshot().map((e) => e.status)).toEqual([
    "active",
    "resolved",
  ]);
  restored.clearHistory();
  restored.resolve("B");
  restored.flush();
  expect(new ClientErrors({ storage: disk }).snapshot()).toHaveLength(1);
  expect(restored.snapshot()[0]).toMatchObject({
    status: "resolved",
    message: "second",
  });
  for (let i = 0; i < 10; i++) restored.report(String(i), new Error(String(i)));
  expect(restored.snapshot()).toHaveLength(3);
  restored.flush();
});
it("keeps dismissed entries distinct from confirmed recovery and tolerates storage/listener failures", () => {
  const errors = new ClientErrors({
    storage: {
      getItem: () => {
        throw Error("denied");
      },
      setItem: () => {
        throw Error("full");
      },
    },
  });
  errors.subscribe(() => {
    throw Error("view");
  });
  const id = errors.report("A", new Error("failure"))!;
  errors.dismiss(id);
  expect(errors.snapshot()[0].status).toBe("dismissed");
  expect(() => errors.flush()).not.toThrow();
  errors.report("A", new Error("failure"));
  expect(errors.snapshot()).toHaveLength(2);
  errors.flush();
});
it("recognizes RPC feedback without hiding ordinary validation messages", () => {
  expect(
    isAPIError(
      "Cannot load: ConnectError: [unavailable] offline",
      new ClientErrors(),
    ),
  ).toBe(true);
  expect(isAPIError("Alias must contain letters", new ClientErrors())).toBe(
    false,
  );
  expect(
    isAPIError("Alias must not contain [not_found]", new ClientErrors()),
  ).toBe(false);
});

it("moves errors into stale history one hour after their last occurrence, including with the dialog closed", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-11-07T00:00:00Z"));
  const errors = new ClientErrors();
  const changed = vi.fn();
  const stop = errors.subscribe(changed);
  const id = errors.report("A", "offline")!;
  const lastSeen = Date.now();
  vi.advanceTimersByTime(errorStaleAfter - 1);
  expect(errors.snapshot()[0].status).toBe("active");
  vi.advanceTimersByTime(1);
  expect(errors.snapshot()[0]).toMatchObject({
    id,
    status: "stale",
    lastSeen,
    count: 1,
    closedAt: lastSeen + errorStaleAfter,
  });
  expect(changed).toHaveBeenCalledTimes(2);
  errors.resolve("A");
  expect(errors.snapshot()[0].status).toBe("stale");
  errors.report("A", "offline");
  expect(errors.snapshot().map((entry) => entry.status)).toEqual([
    "active",
    "stale",
  ]);
  expect(errors.snapshot()[0].id).not.toBe(id);
  stop();
  expect(vi.getTimerCount()).toBe(0);
});
it("repeated failures extend the stale deadline and keep the current episode active", () => {
  vi.useFakeTimers();
  const errors = new ClientErrors();
  const stop = errors.subscribe(() => {});
  const id = errors.report("A", "offline");
  vi.advanceTimersByTime(errorStaleAfter - 1000);
  errors.report("A", "still offline");
  const latest = Date.now();
  vi.advanceTimersByTime(1000);
  expect(errors.snapshot()[0]).toMatchObject({
    id,
    status: "active",
    count: 2,
    lastSeen: latest,
  });
  vi.advanceTimersByTime(errorStaleAfter - 1000);
  expect(errors.snapshot()[0]).toMatchObject({ id, status: "stale", count: 2 });
  stop();
});
it("expires persisted old active records on restore and preserves stale status across tabs and reload", () => {
  vi.useFakeTimers();
  const disk = storage();
  const errors = new ClientErrors({ storage: disk });
  const id = errors.report("A", "offline");
  errors.flush();
  vi.setSystemTime(Date.now() + errorStaleAfter + 1);
  const restored = new ClientErrors({ storage: disk });
  expect(restored.snapshot()[0]).toMatchObject({ id, status: "stale" });
  restored.flush();
  errors.restore();
  expect(errors.snapshot()[0].status).toBe("stale");
  restored.clearHistory();
  restored.flush();
  expect(new ClientErrors({ storage: disk }).snapshot()).toEqual([]);
});

it("dismisses all Active episodes in one update, preserving history, persistence and recurrence", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-11-07T14:00:00Z"));
  const disk = storage();
  const errors = new ClientErrors({ storage: disk });
  errors.report("old", "Old failure");
  vi.advanceTimersByTime(errorStaleAfter + 1);
  errors.report("A", "First failure");
  errors.report("B", "Second failure");
  errors.report("B", "Second failure");
  errors.report("recovered", "Recovered failure");
  errors.resolve("recovered");
  const history = errors
    .snapshot()
    .filter((entry) => entry.status !== "active");
  const active = errors.snapshot().filter((entry) => entry.status === "active");
  const changed = vi.fn();
  const stop = errors.subscribe(changed);
  errors.dismissAll();
  expect(changed).toHaveBeenCalledTimes(1);
  expect(
    errors.snapshot().filter((entry) => entry.status === "dismissed"),
  ).toEqual(
    active.map((entry) => ({
      ...entry,
      status: "dismissed",
      closedAt: Date.now(),
      updated: expect.any(Number),
    })),
  );
  for (const entry of history) expect(errors.snapshot()).toContain(entry);
  expect(errors.snapshot().find((entry) => entry.key === "old")?.status).toBe(
    "stale",
  );
  const snapshot = errors.snapshot();
  errors.dismissAll();
  expect(errors.snapshot()).toBe(snapshot);
  expect(changed).toHaveBeenCalledTimes(1);
  errors.flush();
  const restored = new ClientErrors({ storage: disk });
  expect(restored.snapshot()).toEqual(errors.snapshot());
  errors.report("A", "First failure");
  expect(errors.snapshot()[0]).toMatchObject({
    key: "A",
    status: "active",
    count: 1,
  });
  expect(errors.snapshot()[0].id).not.toBe(
    active.find((entry) => entry.key === "A")!.id,
  );
  stop();
});

it("does not restore persisted Active copies when dismissed history is immediately cleared", () => {
  vi.useFakeTimers();
  const disk = storage();
  const errors = new ClientErrors({ storage: disk });
  errors.report("A", "First failure");
  errors.report("B", "Second failure");
  errors.flush();
  errors.dismissAll();
  errors.clearHistory();
  expect(errors.snapshot()).toHaveLength(0);
  errors.flush();
  expect(errors.snapshot()).toHaveLength(0);
  expect(new ClientErrors({ storage: disk }).snapshot()).toHaveLength(0);
});

it("retains notify:false records through storage while defaults and later unattended reports notify normally", () => {
  vi.useFakeTimers();
  const disk = storage();
  const errors = new ClientErrors({ storage: disk });
  const id = errors.report(
    "read",
    new ConnectError("history missing", Code.NotFound),
    { notify: false },
  )!;
  errors.report("background", "background failure");
  errors.flush();
  const restored = new ClientErrors({ storage: disk });
  expect(restored.snapshot().find((entry) => entry.id === id)).toMatchObject({
    status: "active",
    notify: false,
  });
  expect(
    restored.snapshot().find((entry) => entry.key === "background")?.notify,
  ).not.toBe(false);
  restored.report("read", "history missing");
  expect(restored.snapshot().find((entry) => entry.id === id)).toMatchObject({
    count: 2,
    status: "active",
  });
  expect(restored.snapshot().find((entry) => entry.id === id)?.notify).not.toBe(
    false,
  );
  const data = JSON.parse(disk.getItem(errors.storageKey)!);
  data.entries.forEach(
    (entry: { notify: unknown }) => (entry.notify = "false"),
  );
  disk.setItem(errors.storageKey, JSON.stringify(data));
  expect(
    new ClientErrors({ storage: disk })
      .snapshot()
      .every((entry) => entry.notify !== false),
  ).toBe(true);
});
