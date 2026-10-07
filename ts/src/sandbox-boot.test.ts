import { describe, expect, it, vi } from "vitest";
import { boot, BootTimeout, deadline } from "./sandbox-boot";

describe("sandbox boot", () => {
  it("resolves what finishes in time and clears its timer", async () => {
    const timer = { setTimeout: vi.fn(() => 1), clearTimeout: vi.fn() };
    await expect(
      deadline(Promise.resolve("ready"), 10, timer as never),
    ).resolves.toBe("ready");
    expect(timer.clearTimeout).toHaveBeenCalledWith(1);
  });

  it("stops waiting for work that never finishes", async () => {
    let fire = () => {};
    const timer = {
      setTimeout: (fn: () => void) => {
        fire = fn;
        return 1;
      },
      clearTimeout: vi.fn(),
    };
    const stuck = deadline(new Promise(() => {}), 15000, timer as never);
    fire();
    await expect(stuck).rejects.toBeInstanceOf(BootTimeout);
    // The message has to name the symptom, because the symptom is all there is:
    // no error and no progress is what the page was showing.
    await expect(stuck).rejects.toThrow(/15s/);
  });

  it("keeps a real failure instead of replacing it with a retry", async () => {
    const attempt = vi.fn(async () => {
      throw new Error("GET /app.wasm failed: 404 Not Found");
    });
    await expect(boot(attempt, { ms: 10 })).rejects.toThrow(/404/);
    expect(attempt).toHaveBeenCalledTimes(1);
  });

  it("retries a stall once, without the cache", async () => {
    const retried: boolean[] = [];
    const attempt = async (cached: boolean) => {
      retried.push(cached);
      if (cached) return new Promise<string>(() => {});
      return "ready";
    };
    const notified: BootTimeout[] = [];
    await expect(
      boot(attempt, { ms: 5, onRetry: (e) => notified.push(e) }),
    ).resolves.toBe("ready");
    expect(retried).toEqual([true, false]);
    // The retry is said out loud: a page that silently recovered teaches nobody
    // that the cache is what stalled.
    expect(notified).toHaveLength(1);
  });

  it("reports a second stall rather than retrying forever", async () => {
    let attempts = 0;
    const attempt = async () => {
      attempts++;
      return new Promise<string>(() => {});
    };
    await expect(boot(attempt, { ms: 5 })).rejects.toBeInstanceOf(BootTimeout);
    expect(attempts).toBe(2);
  });

  it("cancels a stalled service before starting a new worker", async () => {
    const calls: string[] = [];
    let first: AbortSignal | undefined;
    const attempt = async (cached: boolean, signal: AbortSignal) => {
      if (!cached) {
        expect(first?.aborted).toBe(true);
        calls.push("retry");
        return "ready";
      }
      first = signal;
      return new Promise<string>((_, reject) => {
        signal.addEventListener("abort", () => {
          calls.push("closed");
          reject(signal.reason);
        });
      });
    };
    await expect(boot(attempt, { ms: 5 })).resolves.toBe("ready");
    expect(calls).toEqual(["closed", "retry"]);
  });
});
