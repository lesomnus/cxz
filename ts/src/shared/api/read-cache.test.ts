import { expect, it, vi } from "vitest";
import { ReadCache, ACCOUNT_INFO_TTL, USAGE_TTL } from "./read-cache";

const signal = () => new AbortController().signal;

it("reuses usage for one minute and metadata for five minutes without extending TTL on access", async () => {
  let now = 0;
  const cache = new ReadCache(8, () => now);
  const usage = vi.fn(async () => ({ turns: 1n }));
  const info = vi.fn(async () => ({ alias: "work" }));
  await cache.read("usage", USAGE_TTL, usage, signal());
  await cache.read("info", ACCOUNT_INFO_TTL, info, signal());
  now = USAGE_TTL - 1;
  await cache.read("usage", USAGE_TTL, usage, signal());
  await cache.read("info", ACCOUNT_INFO_TTL, info, signal());
  expect(usage).toHaveBeenCalledTimes(1);
  now = USAGE_TTL;
  await cache.read("usage", USAGE_TTL, usage, signal());
  await cache.read("info", ACCOUNT_INFO_TTL, info, signal());
  expect(usage).toHaveBeenCalledTimes(2);
  expect(info).toHaveBeenCalledTimes(1);
  now = ACCOUNT_INFO_TTL;
  await cache.read("info", ACCOUNT_INFO_TTL, info, signal());
  expect(info).toHaveBeenCalledTimes(2);
});

it("an invalidated in-flight read cannot overwrite a forced refresh", async () => {
  const cache = new ReadCache();
  let complete!: (value: string) => void;
  const old = cache.read(
    "account/work/usage",
    USAGE_TTL,
    () =>
      new Promise<string>((resolve) => {
        complete = resolve;
      }),
    signal(),
  );
  cache.invalidatePrefix("account/work/");
  await cache.read(
    "account/work/usage",
    USAGE_TTL,
    async () => "new",
    signal(),
  );
  complete("old");
  await old;
  expect(cache.peek("account/work/usage")?.data).toBe("new");
});

it("does not cache failures or aborted reads, and disposal prevents repopulation", async () => {
  const cache = new ReadCache();
  await expect(
    cache.read(
      "failure",
      USAGE_TTL,
      async () => {
        throw new Error("offline");
      },
      signal(),
    ),
  ).rejects.toThrow("offline");
  expect(cache.peek("failure")).toBeUndefined();
  const controller = new AbortController();
  let complete!: (value: string) => void;
  const pending = cache.read(
    "cancel",
    USAGE_TTL,
    () =>
      new Promise<string>((resolve) => {
        complete = resolve;
      }),
    controller.signal,
  );
  controller.abort();
  complete("canceled");
  await pending;
  expect(cache.peek("cancel")).toBeUndefined();
  const disposed = cache.read(
    "dispose",
    USAGE_TTL,
    () =>
      new Promise<string>((resolve) => {
        complete = resolve;
      }),
    signal(),
  );
  cache.clear();
  complete("gone");
  await disposed;
  expect(cache.peek("dispose")).toBeUndefined();
});

it("isolates connections, bounds entries and evicts the least recently viewed result", async () => {
  const cache = new ReadCache(2);
  await cache.read("one", USAGE_TTL, async () => 1, signal());
  await cache.read("two", USAGE_TTL, async () => 2, signal());
  cache.peek("one");
  await cache.read("three", USAGE_TTL, async () => 3, signal());
  expect(cache.peek("one")?.data).toBe(1);
  expect(cache.peek("two")).toBeUndefined();
  expect(new ReadCache().peek("one")).toBeUndefined();
});

it("soft expiry retains the last successful value while a refresh is pending or fails", async () => {
  const cache = new ReadCache();
  await cache.read(
    "background/session/run",
    10_000,
    async () => ["previous"],
    signal(),
  );
  cache.expire("background/session/run");
  expect(cache.peek("background/session/run")).toEqual({
    data: ["previous"],
    fresh: false,
  });
  await expect(
    cache.read(
      "background/session/run",
      10_000,
      async () => {
        throw new Error("offline");
      },
      signal(),
    ),
  ).rejects.toThrow("offline");
  expect(cache.peek("background/session/run")?.data).toEqual(["previous"]);
  await cache.read(
    "background/session/run",
    10_000,
    async () => ["current"],
    signal(),
  );
  expect(cache.peek("background/session/run")).toEqual({
    data: ["current"],
    fresh: true,
  });
});
