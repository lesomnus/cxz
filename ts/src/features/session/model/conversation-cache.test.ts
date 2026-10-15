import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { afterEach, expect, it, vi } from "vitest";
import {
  SessionSchema,
  SessionEventSchema,
  type SessionEvent,
} from "#gen/cxz/session_pb";
import {
  ConversationCache,
  MAX_IDLE_CONVERSATIONS,
  MAX_CONVERSATION_BYTES,
  eventBytes,
} from "./conversation-cache";

const event = (seq: bigint, kind = "assistant", text = `Response ${seq}`) =>
  create(SessionEventSchema, {
    seq,
    kind,
    text,
    runId: "run",
    timeMs: BigInt(Date.now()),
  });
const caches: ConversationCache[] = [];
afterEach(() => {
  for (const cache of caches.splice(0)) cache.dispose();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
function fixture() {
  const channels = new Map<
    string,
    ReadableStreamDefaultController<SessionEvent>
  >();
  const signals = new Map<string, AbortSignal>();
  const sessions = {
    get: vi.fn(async ({ ref }) =>
      create(SessionSchema, {
        id: new TextEncoder().encode(ref.key.value),
        runtimeId: ref.key.value,
        status: { runId: "run", state: "idle", lastSeq: 1n },
      }),
    ),
    transcript: vi.fn(async (_query: { limit: number }) => ({
      events: [event(1n)],
      metadata: [],
      snapshotSeq: 1n,
      hasOlder: false,
      hasNewer: false,
    })),
    history: vi.fn(async (_query: { afterSeq: bigint }) => ({
      events: [] as SessionEvent[],
    })),
    events: vi.fn(({ ref }, { signal }) => {
      const id = ref.key.value;
      signals.set(id, signal);
      const stream = new ReadableStream<SessionEvent>({
        start(c) {
          channels.set(id, c);
        },
      });
      const reader = stream.getReader();
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
    }),
  };
  const attention = { event: vi.fn() };
  const cache = new ConversationCache({
    sessions,
    attention,
    clientId: "tab",
  } as unknown as ConstructorParameters<typeof ConversationCache>[0]);
  caches.push(cache);
  const open = async (id: string) => {
    const feed = cache.get(id);
    const stop = feed.subscribe(() => {});
    await vi.waitFor(() => expect(feed.snapshot().status).toBe("Live"));
    return { feed, stop };
  };
  return { cache, sessions, attention, channels, signals, open };
}
it("keeps the native stream and unseen responses when another session is opened", async () => {
  const f = fixture();
  const a = await f.open("a");
  a.stop();
  const b = await f.open("b");
  f.channels.get("a")!.enqueue(event(2n));
  f.channels.get("a")!.enqueue(event(3n, "turn_end", "completed"));
  await vi.waitFor(() => expect(a.feed.latestSeq.current).toBe(3n));
  expect(f.signals.get("a")!.aborted).toBe(false);
  const again = await f.open("a");
  expect(again.feed).toBe(a.feed);
  expect(again.feed.snapshot().loading).toBeUndefined();
  expect(again.feed.snapshot().events.map((e) => e.seq)).toEqual([1n, 2n, 3n]);
  expect(f.sessions.transcript).toHaveBeenCalledTimes(2);
  expect(f.sessions.events).toHaveBeenCalledTimes(2);
  expect(f.attention.event).toHaveBeenCalledWith(
    "a",
    expect.objectContaining({ seq: 3n }),
  );
  b.stop();
  again.stop();
});
it("retains completed empty history without treating a zero cursor as a new initial load", async () => {
  const f = fixture();
  f.sessions.transcript.mockResolvedValue({
    events: [],
    metadata: [],
    snapshotSeq: 0n,
    hasOlder: false,
    hasNewer: false,
  });
  f.sessions.get.mockResolvedValue(
    create(SessionSchema, {
      id: new Uint8Array([1]),
      status: { runId: "run", state: "idle", lastSeq: 0n },
    }),
  );
  const a = await f.open("a");
  a.stop();
  await f.open("a");
  expect(f.sessions.transcript).toHaveBeenCalledTimes(1);
  expect(a.feed.snapshot().loading).toBeUndefined();
});

it("settles stale catalog approvals before exposing the initial conversation", async () => {
  const f = fixture();
  const approval = {
    ...event(1n, "approval", "Bash"),
    requestId: "permission",
  };
  const resolved = {
    ...event(2n, "approval_resolved"),
    requestId: "permission",
  };
  f.sessions.get.mockResolvedValue(
    create(SessionSchema, {
      id: new TextEncoder().encode("a"),
      status: {
        runId: "run",
        state: "waiting_input",
        lastSeq: 1n,
        pending: [approval],
      },
    }),
  );
  let finish!: (value: { events: SessionEvent[] }) => void;
  f.sessions.history.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const feed = f.cache.get("a");
  const shown: SessionEvent[][] = [];
  feed.subscribe(() => shown.push(feed.snapshot().pending));
  await vi.waitFor(() => expect(f.sessions.history).toHaveBeenCalled());
  // Longer than the UI grace period: loading must not expose the stale request.
  await new Promise((resolve) => setTimeout(resolve, 200));
  expect(feed.snapshot().loading).toBeDefined();
  expect(feed.snapshot().pending).toEqual([]);
  finish({ events: [resolved, event(3n, "state", "idle")] });
  await vi.waitFor(() => expect(feed.snapshot().status).toBe("Live"));
  expect(shown.flat()).toEqual([]);
  expect(feed.snapshot().executionState).toBe("idle");
  expect(f.sessions.events.mock.calls[0][0]).toMatchObject({ afterSeq: 3n });
});

it("keeps genuinely pending requests discovered during initial catch-up", async () => {
  const f = fixture();
  const approval = {
    ...event(2n, "approval", "Bash"),
    requestId: "permission",
  };
  const question = {
    ...event(3n, "approval", "AskUserQuestion"),
    requestId: "question",
  };
  f.sessions.history.mockResolvedValueOnce({ events: [approval, question] });
  const { feed } = await f.open("a");
  expect(feed.snapshot().pending).toEqual([approval, question]);
  f.channels
    .get("a")!
    .enqueue({ ...event(4n, "approval_resolved"), requestId: "permission" });
  await vi.waitFor(() => expect(feed.snapshot().pending).toEqual([question]));
});

it("settles approval lifecycles across short native pages without trusting projected rows", async () => {
  const f = fixture();
  const approval = {
    ...event(2n, "approval", "Bash"),
    requestId: "permission",
  };
  // A projection may retain the approval while folding away its resolution.
  f.sessions.transcript.mockResolvedValue({
    events: [event(1n), approval],
    metadata: [],
    snapshotSeq: 4n,
    hasOlder: false,
    hasNewer: false,
  });
  f.sessions.history.mockResolvedValueOnce({ events: [approval] });
  f.sessions.history.mockResolvedValueOnce({
    events: [
      { ...event(3n, "approval_resolved"), requestId: "permission" },
      event(4n),
    ],
  });
  const { feed } = await f.open("a");
  expect(feed.snapshot().pending).toEqual([]);
  expect(f.sessions.history.mock.calls.map(([q]) => q.afterSeq)).toEqual([
    1n,
    2n,
    4n,
  ]);
  expect(f.sessions.events.mock.calls[0][0]).toMatchObject({ afterSeq: 4n });
});

it("clears a prior run's approval at a resume boundary, during loading and live", async () => {
  const f = fixture();
  const approval = {
    ...event(2n, "approval", "Bash"),
    requestId: "permission",
  };
  f.sessions.history.mockResolvedValueOnce({
    events: [approval, { ...event(3n, "state", "idle"), runId: "new" }],
  });
  const { feed } = await f.open("a");
  expect(feed.snapshot().pending).toEqual([]);
  f.channels.get("a")!.enqueue({ ...approval, seq: 4n, runId: "new" });
  await vi.waitFor(() => expect(feed.snapshot().pending).toHaveLength(1));
  f.channels
    .get("a")!
    .enqueue({ ...event(5n, "state", "idle"), runId: "next" });
  await vi.waitFor(() => expect(feed.snapshot().pending).toEqual([]));
});
it("reconnects at the native cursor and replays an old task's completion without fetching the first transcript again", async () => {
  const f = fixture();
  const a = await f.open("a");
  a.stop();
  f.channels.get("a")!.enqueue(event(2n, "tool_call"));
  await vi.waitFor(() => expect(a.feed.latestSeq.current).toBe(2n));
  f.sessions.get.mockResolvedValue(
    create(SessionSchema, {
      id: new TextEncoder().encode("a"),
      status: { runId: "run", state: "idle", lastSeq: 3n },
    }),
  );
  f.sessions.history.mockResolvedValue({ events: [event(3n, "tool_result")] });
  vi.useFakeTimers();
  f.channels.get("a")!.error(new Error("offline"));
  await vi.advanceTimersByTimeAsync(2000);
  expect(f.sessions.history.mock.calls.at(-1)![0]).toMatchObject({
    afterSeq: 3n,
  });
  expect(f.sessions.history.mock.calls).toContainEqual([
    expect.objectContaining({ afterSeq: 2n }),
    expect.anything(),
  ]);
  expect(f.sessions.events.mock.calls.at(-1)![0]).toMatchObject({
    afterSeq: 3n,
  });
  expect(f.sessions.transcript).toHaveBeenCalledTimes(1);
  expect(a.feed.snapshot().events.map((e) => e.seq)).toEqual([1n, 2n, 3n]);
  expect(a.feed.snapshot().loading).toBeUndefined();
});

it("retries an interrupted catch-up from the committed cursor without losing pending requests", async () => {
  const f = fixture();
  const { feed } = await f.open("a");
  const approval = {
    ...event(2n, "approval", "Bash"),
    requestId: "permission",
  };
  f.sessions.history.mockResolvedValueOnce({ events: [approval] });
  f.sessions.history.mockRejectedValueOnce(new Error("history interrupted"));
  f.sessions.history.mockResolvedValueOnce({ events: [approval] });
  vi.useFakeTimers();
  f.channels.get("a")!.error(new Error("offline"));
  await vi.advanceTimersByTimeAsync(2000);
  expect(feed.snapshot().pending).toEqual([]);
  expect(feed.latestSeq.current).toBe(1n);
  await vi.advanceTimersByTimeAsync(2000);
  expect(feed.snapshot().pending).toEqual([approval]);
  expect(f.sessions.events.mock.calls.at(-1)![0]).toMatchObject({
    afterSeq: 2n,
  });
  // A stale Get on the next reconnect must not erase known live approvals.
  f.sessions.transcript.mockResolvedValue({
    events: [approval],
    metadata: [],
    snapshotSeq: 2n,
    hasOlder: true,
    hasNewer: false,
  });
  f.channels.get("a")!.error(new Error("offline again"));
  await vi.advanceTimersByTimeAsync(2000);
  expect(feed.snapshot().pending).toEqual([approval]);
});
it("freezes a detached reading window while maintaining a latest page and control state", async () => {
  const f = fixture();
  const a = await f.open("a");
  a.feed.setEvents([event(1n, "input")]);
  a.feed.detached.current = true;
  a.feed.follow.current = false;
  a.stop();
  f.channels.get("a")!.enqueue(event(2n, "input"));
  f.channels.get("a")!.enqueue(event(3n));
  f.channels.get("a")!.enqueue(event(4n, "queued", "Next prompt"));
  await vi.waitFor(() => expect(a.feed.latestSeq.current).toBe(4n));
  expect(a.feed.snapshot().events).toHaveLength(1);
  expect(a.feed.latestPage()!.events.map((e) => e.seq)).toEqual([1n, 2n, 3n]);
  expect(a.feed.snapshot().queued).toBe("Next prompt");
});
it("protocol traffic advances the replay cursor without notifying or rebuilding readable rows", async () => {
  const f = fixture();
  const a = await f.open("a");
  const snapshot = a.feed.snapshot();
  f.channels.get("a")!.enqueue(event(2n, "raw"));
  await vi.waitFor(() => expect(a.feed.latestSeq.current).toBe(2n));
  expect(a.feed.snapshot()).toBe(snapshot);
});
it("evicts idle LRU entries, protects viewed and working sessions and disposes their streams on sign-out", async () => {
  const f = fixture();
  const working = await f.open("working");
  f.channels.get("working")!.enqueue(event(2n, "input"));
  await vi.waitFor(() => expect(working.feed.active).toBe(true));
  working.stop();
  const viewed = await f.open("viewed");
  const old = await f.open("old");
  old.stop();
  for (let i = 0; i < MAX_IDLE_CONVERSATIONS; i++) {
    const next = await f.open(`idle-${i}`);
    next.stop();
  }
  expect(f.signals.get("old")!.aborted).toBe(true);
  expect(f.signals.get("working")!.aborted).toBe(false);
  expect(f.signals.get("viewed")!.aborted).toBe(false);
  f.cache.dispose();
  expect([...f.signals.values()].every((s) => s.aborted)).toBe(true);
  expect(working.feed.snapshot().events).toHaveLength(0);
  viewed.stop();
});
it("limits large live bodies by bytes as well as record count", async () => {
  const f = fixture();
  const a = await f.open("a");
  const text = "x".repeat(MAX_CONVERSATION_BYTES / 4);
  for (let seq = 2n; seq <= 5n; seq++)
    f.channels.get("a")!.enqueue(event(seq, "assistant", text));
  await vi.waitFor(() => expect(a.feed.latestSeq.current).toBe(5n));
  expect(a.feed.snapshot().events.length).toBeLessThan(5);
  expect(a.feed.snapshot().events.at(-1)!.seq).toBe(5n);
});
it.each(["older", "newer"] as const)(
  "retains a contiguous reading window when a large %s page exceeds the budget",
  async (edge) => {
    const f = fixture();
    const { feed } = await f.open("a");
    const rows = Array.from({ length: 24 }, (_, index) =>
      event(BigInt(index + 1)),
    );
    for (const e of rows)
      e.payload = new Uint8Array(MAX_CONVERSATION_BYTES / 8);
    feed.follow.current = false;
    feed.viewport.anchor = { id: "12", offset: 18 };
    feed.setEvents(rows, edge);
    const retained = feed.snapshot().events;
    expect(retained.some((e) => e.seq === 12n)).toBe(true);
    expect(retained.some((e) => e.seq < 12n)).toBe(true);
    expect(retained.some((e) => e.seq > 12n)).toBe(true);
    expect(retained.reduce((n, e) => n + eventBytes(e), 0)).toBeLessThanOrEqual(
      MAX_CONVERSATION_BYTES,
    );
    expect(
      retained.every((e, i) => !i || e.seq === retained[i - 1].seq + 1n),
    ).toBe(true);
    expect(feed.viewport.anchor).toEqual({ id: "12", offset: 18 });
    feed.setEvents(rows, edge);
    expect(feed.snapshot().events).toBe(retained);
  },
);
it("drops a recreated session's old journal before loading its new identity", async () => {
  const f = fixture();
  const a = await f.open("a");
  a.stop();
  f.sessions.get.mockResolvedValue(
    create(SessionSchema, {
      id: new TextEncoder().encode("new-a"),
      status: { runId: "new", state: "idle", lastSeq: 1n },
    }),
  );
  f.sessions.transcript.mockResolvedValue({
    events: [event(1n, "input", "New journal")],
    metadata: [],
    snapshotSeq: 1n,
    hasOlder: false,
    hasNewer: false,
  });
  vi.useFakeTimers();
  f.channels.get("a")!.error(new Error("recreate"));
  await vi.advanceTimersByTimeAsync(2000);
  expect(f.sessions.transcript).toHaveBeenCalledTimes(2);
  expect(a.feed.snapshot().events.map((e) => e.text)).toEqual(["New journal"]);
});

it("does not erase received responses when catalog LastSeq lags the journal on reconnect", async () => {
  const f = fixture();
  const a = await f.open("a");
  a.stop();
  f.channels.get("a")!.enqueue(event(2n));
  await vi.waitFor(() => expect(a.feed.latestSeq.current).toBe(2n));
  // Get still reports 1; the journal itself has already persisted 2.
  f.sessions.transcript.mockResolvedValue({
    events: [event(2n)],
    metadata: [],
    snapshotSeq: 2n,
    hasOlder: false,
    hasNewer: false,
  });
  vi.useFakeTimers();
  f.channels.get("a")!.error(new Error("offline"));
  await vi.advanceTimersByTimeAsync(2000);
  expect(f.sessions.transcript.mock.calls.at(-1)![0]).toMatchObject({
    limit: 1,
  });
  expect(a.feed.snapshot().events.map((e) => e.seq)).toEqual([1n, 2n]);
  expect(a.feed.snapshot().loading).toBeUndefined();
  expect(f.sessions.events.mock.calls.at(-1)![0]).toMatchObject({
    afterSeq: 2n,
  });
});

it("coalesces visible updates into one frame and schedules no painting for an offscreen stream", async () => {
  const frames: FrameRequestCallback[] = [];
  const raf = vi.fn((cb: FrameRequestCallback) => {
    frames.push(cb);
    return frames.length;
  });
  vi.stubGlobal("requestAnimationFrame", raf);
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
  const f = fixture();
  const a = await f.open("a");
  for (const cb of frames.splice(0)) cb(0);
  raf.mockClear();
  for (let seq = 2n; seq <= 4n; seq++) f.channels.get("a")!.enqueue(event(seq));
  await vi.waitFor(() => expect(a.feed.latestSeq.current).toBe(4n));
  expect(raf).toHaveBeenCalledTimes(1);
  for (const cb of frames.splice(0)) cb(0);
  a.stop();
  raf.mockClear();
  f.channels.get("a")!.enqueue(event(5n));
  await vi.waitFor(() => expect(a.feed.latestSeq.current).toBe(5n));
  expect(raf).not.toHaveBeenCalled();
});

it("stops an externally removed session, then checks its identity when it is reopened", async () => {
  const f = fixture();
  const a = await f.open("a");
  f.channels.get("a")!.enqueue(event(2n, "input"));
  await vi.waitFor(() => expect(a.feed.active).toBe(true));
  a.stop();
  f.sessions.get.mockRejectedValue(new ConnectError("Purged", Code.NotFound));
  vi.useFakeTimers();
  f.channels.get("a")!.error(new Error("closed"));
  await vi.advanceTimersByTimeAsync(2000);
  expect(a.feed.active).toBe(false);
  const requests = f.sessions.get.mock.calls.length;
  await vi.advanceTimersByTimeAsync(5000);
  expect(f.sessions.get).toHaveBeenCalledTimes(requests);
  f.sessions.get.mockResolvedValue(
    create(SessionSchema, {
      id: new TextEncoder().encode("new-a"),
      status: { runId: "new", state: "idle", lastSeq: 1n },
    }),
  );
  f.sessions.transcript.mockResolvedValue({
    events: [event(1n, "input", "Recreated")],
    metadata: [],
    snapshotSeq: 1n,
    hasOlder: false,
    hasNewer: false,
  });
  a.feed.subscribe(() => {});
  await vi.advanceTimersByTimeAsync(0);
  expect(a.feed.snapshot().events.map((e) => e.text)).toEqual(["Recreated"]);
});

it("includes structured summary bodies in the cache byte budget", async () => {
  const f = fixture();
  const a = await f.open("a");
  for (let seq = 2n; seq <= 3n; seq++)
    f.channels.get("a")!.enqueue(
      create(SessionEventSchema, {
        seq,
        runId: "run",
        kind: "tool_call",
        toolSummary: {
          name: "Bash",
          command: "x".repeat(MAX_CONVERSATION_BYTES / 3),
        },
      }),
    );
  await vi.waitFor(() => expect(a.feed.latestSeq.current).toBe(3n));
  expect(a.feed.snapshot().events).toHaveLength(1);
  expect(a.feed.snapshot().events[0].seq).toBe(3n);
});
