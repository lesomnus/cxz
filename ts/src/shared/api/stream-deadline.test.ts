import { describe, expect, it, vi } from "vitest";
import { alive } from "./stream-deadline";

const frames = (...values: { beat?: boolean; text?: string }[]) =>
  (async function* () {
    for (const v of values) yield v;
    await new Promise(() => {}); // Open, and then silent.
  })();

describe("reading a stream that says it is alive", () => {
  // Both streams of a conversation are heartbeated by the same runtime, so one
  // reader serves both: the row stream says it with a row that has no event,
  // the record stream with a frame that says so. See cxz#188 and cxz#191.
  it("yields frames, drops heartbeats and reports silence after one is seen", async () => {
    vi.useFakeTimers();
    try {
      const seen: string[] = [];
      const read = (async () => {
        for await (const v of alive(
          frames({ text: "first" }, { beat: true }, { text: "second" }),
          (v) => !!v.beat,
          1000,
          "the stream stopped responding",
        ))
          seen.push(v.text!);
      })();
      const reported = expect(read).rejects.toThrow("stopped responding");
      await vi.advanceTimersByTimeAsync(1000);
      await reported;
      expect(seen).toEqual(["first", "second"]);
    } finally {
      vi.useRealTimers();
    }
  });

  it("waits indefinitely on a sender that never says it is alive", async () => {
    vi.useFakeTimers();
    try {
      let settled = false;
      const read = (async () => {
        for await (const _ of alive(
          frames({ text: "only" }),
          (v) => !!v.beat,
          1000,
          "the stream stopped responding",
        ))
          void _;
      })().then(
        () => (settled = true),
        () => (settled = true),
      );
      await vi.advanceTimersByTimeAsync(10000);
      await Promise.race([read, Promise.resolve()]);
      expect(settled).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });

  it("ends quietly when the sender ends", async () => {
    const seen: string[] = [];
    for await (const v of alive(
      (async function* () {
        yield { text: "a" };
        yield { beat: true };
        yield { text: "b" };
      })(),
      (v) => !!v.beat,
      1000,
      "unused",
    ))
      seen.push(v.text!);
    expect(seen).toEqual(["a", "b"]);
  });
});
