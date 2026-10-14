import { create } from "@bufbuild/protobuf";
import { afterEach, expect, it, vi } from "vitest";
import { AuxStateSchema, type AuxState } from "#gen/cxz/session_svc_pb";
import {
  ConversationCache,
  MAX_IDLE_CONVERSATIONS,
  MAX_IDLE_CONVERSATION_BYTES,
} from "./conversation-cache";

const summary = (text: string, state = "completed") =>
  create(AuxStateSchema, {
    summaries: [{ runId: "run", turn: 3n, responseSeq: 2n, text }],
    current: { state },
  });
const caches: ConversationCache[] = [];
afterEach(() => {
  caches.splice(0).forEach((cache) => cache.dispose());
  vi.useRealTimers();
});
function fixture() {
  const channels = new Map<string, ReadableStreamDefaultController<AuxState>>();
  const signals: AbortSignal[] = [];
  const auxEvents = vi.fn(({ ref }, { signal }) => {
    const id = ref.key.value;
    signals.push(signal);
    const reader = new ReadableStream<AuxState>({
      start(c) {
        channels.set(id, c);
      },
    }).getReader();
    return (async function* () {
      const cancel = () => {
        void reader.cancel();
      };
      signal.addEventListener("abort", cancel, { once: true });
      try {
        while (!signal.aborted) {
          const next = await reader.read();
          if (next.done) return;
          yield next.value;
        }
      } finally {
        signal.removeEventListener("abort", cancel);
        reader.releaseLock();
      }
    })();
  });
  const cache = new ConversationCache({
    sessions: { auxEvents },
    clientId: "tab",
    attention: {},
  } as unknown as ConstructorParameters<typeof ConversationCache>[0]);
  caches.push(cache);
  return { cache, channels, signals, auxEvents };
}
it("shares one stream between views and keeps pushed summaries while neither view is mounted", async () => {
  const f = fixture(),
    a = f.cache.get("a");
  const first = a.aux.subscribe(() => {}),
    controls = a.aux.subscribe(() => {});
  f.channels.get("a")!.enqueue(summary("Initial summary"));
  await vi.waitFor(() =>
    expect(a.aux.snapshot()?.summaries[0].text).toBe("Initial summary"),
  );
  first();
  controls();
  expect(f.signals[0].aborted).toBe(false);
  const b = f.cache.get("b").aux.subscribe(() => {});
  f.channels.get("a")!.enqueue(summary("Produced while viewing b"));
  await vi.waitFor(() =>
    expect(a.aux.snapshot()?.summaries[0].text).toBe(
      "Produced while viewing b",
    ),
  );
  const again = f.cache.get("a").aux;
  expect(again).toBe(a.aux);
  expect(again.snapshot()?.summaries[0].text).toBe("Produced while viewing b");
  const stop = again.subscribe(() => {});
  expect(
    f.auxEvents.mock.calls.filter(([q]) => q.ref.key.value === "a"),
  ).toHaveLength(1);
  stop();
  b();
});
it("keeps the last snapshot during failed streams and accepts an authoritative empty snapshot on retry", async () => {
  vi.useFakeTimers();
  const f = fixture(),
    a = f.cache.get("a").aux;
  const stop = a.subscribe(() => {});
  f.channels.get("a")!.enqueue(summary("Persisted summary"));
  await vi.advanceTimersByTimeAsync(0);
  const last = a.snapshot();
  f.channels.get("a")!.error(new Error("offline"));
  await vi.advanceTimersByTimeAsync(5000);
  expect(f.auxEvents).toHaveBeenCalledTimes(2);
  expect(a.snapshot()).toBe(last);
  f.channels.get("a")!.enqueue(create(AuxStateSchema));
  await vi.advanceTimersByTimeAsync(0);
  expect(a.snapshot()?.summaries).toEqual([]);
  stop();
});
it("pauses with its connection, resumes without blanking, and cannot be overwritten by an aborted stream", async () => {
  const f = fixture(),
    a = f.cache.get("a");
  const stop = a.aux.subscribe(() => {});
  f.channels.get("a")!.enqueue(summary("Before sleep"));
  await vi.waitFor(() => expect(a.aux.snapshot()).toBeDefined());
  const last = a.aux.snapshot();
  a.pause();
  expect(f.signals[0].aborted).toBe(true);
  a.resume();
  expect(a.aux.snapshot()).toBe(last);
  expect(f.auxEvents).toHaveBeenCalledTimes(2);
  f.channels.get("a")!.enqueue(summary("After wake"));
  await vi.waitFor(() =>
    expect(a.aux.snapshot()?.summaries[0].text).toBe("After wake"),
  );
  a.aux.reset();
  expect(a.aux.snapshot()).toBeUndefined();
  expect(f.signals[1].aborted).toBe(true);
  expect(f.auxEvents).toHaveBeenCalledTimes(3);
  stop();
  f.cache.delete("a");
  expect(f.signals[2].aborted).toBe(true);
  expect(a.aux.snapshot()).toBeUndefined();
});
it("protects running aux tasks from idle eviction, then releases completed tasks and their streams", async () => {
  const f = fixture(),
    a = f.cache.get("a");
  const stop = a.aux.subscribe(() => {});
  f.channels.get("a")!.enqueue(summary("", "running"));
  await vi.waitFor(() => expect(a.active).toBe(true));
  stop();
  for (let i = 0; i <= MAX_IDLE_CONVERSATIONS; i++) {
    const release = f.cache.get(`idle-${i}`).aux.subscribe(() => {});
    release();
  }
  expect(f.signals[0].aborted).toBe(false);
  f.channels.get("a")!.enqueue(summary("Done"));
  await vi.waitFor(() => expect(f.signals[0].aborted).toBe(true));
  expect(a.aux.snapshot()).toBeUndefined();
});
it("counts aux snapshots against the idle byte budget and releases them on eviction", async () => {
  const f = fixture(),
    a = f.cache.get("large");
  const stop = a.aux.subscribe(() => {});
  f.channels
    .get("large")!
    .enqueue(summary("x".repeat(MAX_IDLE_CONVERSATION_BYTES / 2 + 1)));
  await vi.waitFor(() =>
    expect(a.bytes).toBeGreaterThan(MAX_IDLE_CONVERSATION_BYTES),
  );
  stop();
  expect(f.signals[0].aborted).toBe(true);
  expect(a.aux.snapshot()).toBeUndefined();
});
