import { create } from "@bufbuild/protobuf";
import { expect, it, vi } from "vitest";
import {
  SessionSchema,
  SessionStatusSchema,
  SessionEventSchema,
} from "#gen/cxz/session_pb";
import { SessionAttention } from "./session-attention";

function fixture() {
  const history = vi.fn().mockResolvedValue({ events: [] });
  const data = new Map<string, string>();
  const storage = () =>
    ({
      getItem: (k: string) => data.get(k) ?? null,
      setItem: (k: string, v: string) => {
        data.set(k, v);
      },
      removeItem: (k: string) => {
        data.delete(k);
      },
    }) as Storage;
  const tracker = new SessionAttention({ history }, "host", storage);
  const alert = vi.fn();
  tracker.onAlert(alert);
  return { tracker, alert, history, storage };
}
function session(seq: number, state = "idle", run = "run") {
  return create(SessionSchema, {
    runtimeId: "session",
    agent: "claude",
    status: create(SessionStatusSchema, {
      runId: run,
      lastSeq: BigInt(seq),
      state,
    }),
  });
}
const event = (seq: number, text = "completed", runId = "run") =>
  create(SessionEventSchema, {
    seq: BigInt(seq),
    runId,
    kind: "turn_end",
    text,
  });

it("establishes a silent baseline and confirms coalesced idle completions once", async () => {
  const f = fixture();
  f.tracker.observe([session(10)]);
  expect(f.history).not.toHaveBeenCalled();
  expect(f.alert).not.toHaveBeenCalled();
  f.history.mockResolvedValue({ events: [event(12)] });
  f.tracker.observe([session(13)]);
  await vi.waitFor(() => expect(f.alert).toHaveBeenCalledWith("complete"));
  expect([...f.tracker.snapshot()]).toEqual(["session"]);
  f.tracker.observe([session(13)]);
  f.tracker.event("session", event(12));
  expect(f.alert).toHaveBeenCalledTimes(1);
  f.tracker.setReading("session", 11n);
  expect(f.tracker.snapshot().size).toBe(1);
  f.tracker.setReading("session", 13n);
  expect(f.tracker.snapshot().size).toBe(0);
});
it("does not mark idle errors, interruptions, quota changes or old-run replies as completed", async () => {
  const f = fixture();
  f.tracker.observe([session(10, "working")]);
  f.history.mockResolvedValue({
    events: [
      event(11, "interrupted"),
      event(12, "completed", "older-run"),
      create(SessionEventSchema, {
        seq: 13n,
        kind: "usage_status",
        runId: "run",
      }),
    ],
  });
  f.tracker.observe([session(14)]);
  await vi.waitFor(() => expect(f.history).toHaveBeenCalled());
  await new Promise((r) => setTimeout(r, 0));
  expect(f.alert).not.toHaveBeenCalled();
  expect(f.tracker.snapshot().size).toBe(0);
});
it("sounds for live completion even when read, but does not leave an unread marker", () => {
  const f = fixture();
  f.tracker.observe([session(10)]);
  f.tracker.setReading("session", 12n);
  f.tracker.event("session", event(12));
  expect(f.alert).toHaveBeenCalledWith("complete");
  expect(f.tracker.snapshot().size).toBe(0);
});
it("alerts once per new question without replaying the initial pending snapshot", () => {
  const f = fixture();
  const q = create(SessionEventSchema, {
    seq: 11n,
    runId: "run",
    kind: "approval",
    text: "AskUserQuestion",
    requestId: "question-one",
    payload: new TextEncoder().encode(
      JSON.stringify({
        input: { questions: [{ question: "What next?", options: [] }] },
      }),
    ),
  });
  const s = session(12, "working");
  s.status!.pending = [q];
  f.tracker.observe([s]);
  expect(f.alert).not.toHaveBeenCalled();
  s.status!.lastSeq = 13n;
  s.status!.pending = [q, { ...q, requestId: "question-two", seq: 13n }];
  f.tracker.observe([s]);
  f.tracker.observe([s]);
  expect(f.alert.mock.calls).toEqual([["question"]]);
});
it("restores unread and deduplication cursors after refresh", () => {
  const f = fixture();
  f.tracker.observe([session(10)]);
  f.tracker.event("session", event(12));
  const reload = new SessionAttention(
    { history: f.history },
    "host",
    f.storage,
  );
  const sound = vi.fn();
  reload.onAlert(sound);
  expect(reload.snapshot().has("session")).toBe(true);
  reload.event("session", event(12));
  expect(sound).not.toHaveBeenCalled();
  reload.setReading("session", 12n);
  expect(reload.snapshot().size).toBe(0);
});
it("clears stale attention after an ephemeral backend reset instead of keeping an unread reply that no longer exists", () => {
  const f = fixture();
  f.tracker.observe([session(10)]);
  f.tracker.event("session", event(12));
  const reload = new SessionAttention(
    { history: f.history },
    "host",
    f.storage,
  );
  reload.observe([session(5)]);
  expect(reload.snapshot().size).toBe(0);
});
it("caps simultaneous history probes and cancels them on disposal", async () => {
  const f = fixture();
  f.history.mockImplementation(
    (_req, { signal }) =>
      new Promise((_resolve, reject) =>
        signal.addEventListener("abort", () => reject(Error("aborted")), {
          once: true,
        }),
      ),
  );
  const sessions = Array.from({ length: 10 }, (_, i) => ({
    ...session(10),
    runtimeId: `session-${i}`,
  }));
  f.tracker.observe(sessions);
  f.tracker.observe(
    sessions.map((s) => ({ ...s, status: { ...s.status!, lastSeq: 20n } })),
  );
  expect(f.history).toHaveBeenCalledTimes(4);
  f.tracker.dispose();
  await new Promise((r) => setTimeout(r, 0));
  expect(f.history).toHaveBeenCalledTimes(4);
});
