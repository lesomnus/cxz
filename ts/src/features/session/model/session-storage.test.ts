import { describe, expect, it, vi } from "vitest";
import { formatBytes, loadSessionSizes } from "./session-storage";

describe("formatBytes", () => {
  it("reads down a column", () => {
    // No decimal below a kibibyte, where one would be noise, and none past 100
    // where the digit cannot change a comparison.
    expect(formatBytes(0n)).toBe("0 B");
    expect(formatBytes(999n)).toBe("999 B");
    expect(formatBytes(1024n)).toBe("1.0 KiB");
    expect(formatBytes(43_279_457n)).toBe("41.3 MiB");
    expect(formatBytes(888_312_291n)).toBe("847 MiB");
    expect(formatBytes(2_000_000_000n)).toBe("1.9 GiB");
  });
});

describe("loadSessionSizes", () => {
  const reply = (n: number) => ({
    journalBytes: BigInt(n),
    providerBytes: BigInt(n * 2),
    profileBytes: BigInt(n * 3),
    totalBytes: BigInt(n * 6),
  });

  it("asks for each session and keys the answers by runtime id", async () => {
    const storage = vi.fn(async (r: { key: { value: string } }) =>
      reply(r.key.value.length),
    );
    const out = await loadSessionSizes({ storage } as never, [
      { runtimeId: "aa" },
      { runtimeId: "bbb" },
    ]);
    expect(out.get("aa")?.total).toBe(12n);
    expect(out.get("bbb")?.provider).toBe(6n);
    expect(storage).toHaveBeenCalledTimes(2);
  });

  it("holds the number of walks down regardless of how long the list is", async () => {
    let live = 0;
    let peak = 0;
    const storage = vi.fn(async () => {
      peak = Math.max(peak, ++live);
      await new Promise((r) => setTimeout(r, 1));
      live--;
      return reply(1);
    });
    const sessions = Array.from({ length: 20 }, (_, i) => ({
      runtimeId: `s${i}`,
    }));
    const out = await loadSessionSizes({ storage } as never, sessions, {
      concurrency: 3,
    });
    expect(out.size).toBe(20);
    expect(peak).toBeLessThanOrEqual(3);
  });

  it("leaves one unmeasurable session's cell empty rather than the column", async () => {
    const storage = vi.fn(async (r: { key: { value: string } }) => {
      if (r.key.value === "gone") throw new Error("not found");
      return reply(1);
    });
    const out = await loadSessionSizes({ storage } as never, [
      { runtimeId: "gone" },
      { runtimeId: "here" },
    ]);
    expect(out.has("gone")).toBe(false);
    expect(out.get("here")?.total).toBe(6n);
  });

  it("stops asking once the list it was reading is gone", async () => {
    const control = new AbortController();
    const storage = vi.fn(async () => {
      control.abort();
      return reply(1);
    });
    await loadSessionSizes(
      { storage } as never,
      Array.from({ length: 10 }, (_, i) => ({ runtimeId: `s${i}` })),
      { signal: control.signal, concurrency: 1 },
    );
    expect(storage).toHaveBeenCalledTimes(1);
  });
});
