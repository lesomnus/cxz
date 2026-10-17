import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it, vi } from "vitest";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import {
  SessionHistory,
  HISTORY_PAGE_SIZE,
  NATIVE_HISTORY_PAGE_SIZE,
  ROW_STALL_MS,
} from "./session-history";
import type { Connection } from "#src/shared/api/connection.ts";
const event = (seq: bigint, kind = "input") =>
  create(SessionEventSchema, { seq, kind });
const signal = () => new AbortController().signal;
function source(methods: Record<string, unknown>) {
  return new SessionHistory(methods as Connection["sessions"], "s", "c1");
}
describe("historical transcript transport", () => {
  it("uses display rows and a native stream fence, with details fetched separately", async () => {
    const page = {
      events: [event(20n)],
      metadata: [],
      snapshotSeq: 1000n,
      hasOlder: true,
      hasNewer: false,
    };
    const transcript = vi.fn().mockResolvedValue(page);
    const history = vi.fn();
    const eventDetails = vi.fn().mockResolvedValue({
      events: [event(20n, "tool_call"), event(900n, "tool_result")],
    });
    const s = source({ transcript, history, eventDetails });
    const result = await s.page({ snapshotSeq: 1000n }, signal());
    expect(result.snapshotSeq).toBe(1000n);
    expect(result.events.at(-1)?.seq).toBe(20n);
    expect(transcript.mock.calls[0][0]).toMatchObject({
      limit: HISTORY_PAGE_SIZE,
      snapshotSeq: 1000n,
    });
    expect(history).not.toHaveBeenCalled();
    expect(eventDetails).not.toHaveBeenCalled();
    expect((await s.details(20n, signal())).at(-1)?.seq).toBe(900n);
    await s.page({ beforeSeq: 20n, snapshotSeq: 1000n }, signal());
    expect(transcript.mock.calls[1][0]).toMatchObject({ beforeSeq: 20n });
  });
  // A runtime that cannot serve the projection is reported rather than paged
  // natively and reduced here. That fallback is why this client used to carry a
  // second copy of the server's reduction; see cxz#155.
  it("says which side is behind when a runtime lacks the projection", async () => {
    const history = vi.fn();
    const s = source({
      transcript: vi
        .fn()
        .mockRejectedValue(new ConnectError("old runtime", Code.Unimplemented)),
      history,
    });
    await expect(s.page({}, signal())).rejects.toThrow(
      "project's runtime is older than the manager",
    );
    expect(history).not.toHaveBeenCalled();
  });
  // Rows resume from a version cursor, and a row arrives again when a later
  // record corrects it. The caller replaces by seq; nothing is folded here.
  it("streams row versions and carries the cursor, not the sequence", async () => {
    const answer = create(SessionEventSchema, { seq: 4n, kind: "assistant" });
    const rows = vi.fn(async function* () {
      yield { event: answer, updated: 4n };
      yield { event: answer, updated: 5n };
      yield { updated: 6n };
    });
    const s = source({ rows });
    const seen: bigint[] = [];
    for await (const row of s.rows(3n, signal())) seen.push(row.updated);
    expect(seen).toEqual([4n, 5n]);
    expect(rows).toHaveBeenCalledWith(
      expect.objectContaining({ afterUpdated: 3n, clientId: "c1" }),
      expect.anything(),
    );
  });
  // The hole in cxz#188: a stream that stays open and says nothing ends no
  // race and throws nothing, so the conversation stopped growing while the
  // client still read as live. The runtime's heartbeat is what makes silence
  // reportable, and reporting it is what reconnects.
  it("reports a row stream that goes quiet after telling it would speak up", async () => {
    vi.useFakeTimers();
    try {
      const s = source({
        rows: vi.fn(async function* () {
          yield { event: event(4n), updated: 4n };
          yield { updated: 4n };
          await new Promise(() => {});
        }),
      });
      const seen: bigint[] = [];
      const read = (async () => {
        for await (const row of s.rows(0n, signal())) seen.push(row.updated);
      })();
      const reported = expect(read).rejects.toThrow("stopped responding");
      await vi.advanceTimersByTimeAsync(ROW_STALL_MS);
      await reported;
      expect(seen).toEqual([4n]);
    } finally {
      vi.useRealTimers();
    }
  });
  // A runtime that predates heartbeats is legitimately silent. Timing it out
  // would reconnect a healthy conversation forever, so the deadline is armed by
  // a heartbeat rather than by opening the stream.
  it("waits indefinitely on a runtime that never says it is alive", async () => {
    vi.useFakeTimers();
    try {
      const s = source({
        rows: vi.fn(async function* () {
          yield { event: event(4n), updated: 4n };
          await new Promise(() => {});
        }),
      });
      let settled = false;
      const read = (async () => {
        for await (const _ of s.rows(0n, signal())) void _;
      })().then(
        () => (settled = true),
        () => (settled = true),
      );
      await vi.advanceTimersByTimeAsync(ROW_STALL_MS * 10);
      await Promise.race([read, Promise.resolve()]);
      expect(settled).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });
  it("says which side is behind when a runtime cannot stream rows", async () => {
    const s = source({
      rows: vi.fn(async function* () {
        throw new ConnectError("no rows", Code.Unimplemented);
      }),
    });
    await expect(async () => {
      for await (const _ of s.rows(0n, signal())) void _;
    }).rejects.toThrow("project's runtime is older than the manager");
  });
  it("never hides authorization, cancellation or network errors behind a history scan", async () => {
    const history = vi.fn();
    const s = source({
      transcript: vi
        .fn()
        .mockRejectedValue(new ConnectError("denied", Code.PermissionDenied)),
      history,
    });
    await expect(s.page({}, signal())).rejects.toThrow("denied");
    expect(history).not.toHaveBeenCalled();
  });
});
