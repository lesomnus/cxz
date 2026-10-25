import { afterEach, expect, it, vi } from "vitest";
import { ClientLogs } from "./client-logs";
import { MemoryLogStorage } from "./log-storage";

afterEach(() => vi.useRealTimers());
const event = {
  level: "info",
  category: "client",
  message: "observed",
} as const;
it("batches writes, flushes before reads and clears without poisoning later writes", async () => {
  vi.useFakeTimers();
  const storage = new MemoryLogStorage();
  const append = vi.spyOn(storage, "append");
  const logs = new ClientLogs(storage, "test-revision");
  logs.write(event);
  logs.write(event);
  expect(append).not.toHaveBeenCalled();
  const page = await logs.query();
  expect(append).toHaveBeenCalledOnce();
  expect(page.total).toBe(2);
  expect(page.entries[0]).toMatchObject({
    version: "test-revision",
    tab: logs.tab,
  });
  await logs.clear();
  expect((await logs.query()).total).toBe(0);
  logs.write(event);
  await vi.advanceTimersByTimeAsync(500);
  expect((await logs.query()).total).toBe(1);
});
it("bounds a burst before it reaches a slow storage queue and falls back on storage failure", async () => {
  const storage = new MemoryLogStorage();
  vi.spyOn(storage, "append").mockRejectedValue(new Error("storage denied"));
  const logs = new ClientLogs(storage);
  for (let i = 0; i < 500; i++) logs.write({ ...event, message: String(i) });
  const page = await logs.query();
  expect(page.storage).toBe("memory");
  expect(page.total).toBe(257);
  expect(page.entries[0].message).toBe("499");
  logs.subscribe(() => {
    throw new Error("view unavailable");
  });
  logs.write(event);
  await logs.flush();
  expect((await logs.query()).total).toBe(258);
});

it("keeps only one write in flight and a bounded buffer while storage is stalled", async () => {
  const storage = new MemoryLogStorage();
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const append = storage.append.bind(storage);
  const write = vi
    .spyOn(storage, "append")
    .mockImplementationOnce(async (entries) => {
      await gate;
      await append(entries);
    });
  const logs = new ClientLogs(storage);
  logs.write(event);
  const draining = logs.flush();
  await Promise.resolve();
  for (let i = 0; i < 500; i++)
    logs.write({ ...event, message: `while blocked ${i}` });
  expect(logs.flush()).toBe(draining);
  expect(write).toHaveBeenCalledOnce();
  release();
  await draining;
  expect(write).toHaveBeenCalledTimes(2);
  const page = await logs.query();
  expect(page.total).toBe(258);
  expect(await logs.export({ search: "records dropped" })).toContain(
    '"count":244',
  );
});
