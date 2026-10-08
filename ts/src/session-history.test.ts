import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it, vi } from "vitest";
import { SessionEventSchema } from "../gen/cxz/session_pb";
import {
  SessionHistory,
  HISTORY_PAGE_SIZE,
  NATIVE_HISTORY_PAGE_SIZE,
} from "./session-history";
import type { Connection } from "./connection";
const event = (seq: bigint, kind = "input") =>
  create(SessionEventSchema, { seq, kind });
const signal = () => new AbortController().signal;
function source(methods: Record<string, unknown>) {
  return new SessionHistory(methods as Connection["sessions"], "s");
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
  it("uses legacy pages only when this runtime lacks the new API", async () => {
    const transcript = vi
      .fn()
      .mockRejectedValue(new ConnectError("old runtime", Code.Unimplemented));
    const history = vi.fn(async ({ afterSeq }: { afterSeq: bigint }) => ({
      events: Array.from({ length: 128 }, (_, i) =>
        event(afterSeq + BigInt(i + 1)),
      ),
    }));
    const s = source({ transcript, history });
    const p = await s.page({ snapshotSeq: 10000n }, signal());
    expect(p.events).toHaveLength(NATIVE_HISTORY_PAGE_SIZE);
    expect(p.events[0].seq).toBe(8977n);
    expect(p.events.at(-1)?.seq).toBe(10000n);
    expect(p.hasNewer).toBe(false);
    expect(history).toHaveBeenCalledTimes(8);
    const older = await s.page(
      { beforeSeq: 8977n, snapshotSeq: 10000n },
      signal(),
    );
    expect(older.events.at(-1)?.seq).toBe(8976n);
    expect(transcript).toHaveBeenCalledTimes(1);
  });
  it("fetches a larger native window in one round trip when the server honors the limit", async () => {
    const history = vi.fn(async ({ afterSeq, limit }) => ({
      events: Array.from({ length: limit }, (_, i) =>
        event(afterSeq + BigInt(i + 1)),
      ),
    }));
    const s = source({
      transcript: vi
        .fn()
        .mockRejectedValue(
          new ConnectError("old projection", Code.Unimplemented),
        ),
      history,
    });
    const p = await s.page({ snapshotSeq: 10000n }, signal());
    expect(history).toHaveBeenCalledTimes(1);
    expect(p.events).toHaveLength(NATIVE_HISTORY_PAGE_SIZE);
    expect(p.events.at(-1)?.seq).toBe(10000n);
    const newer = await s.page(
      { afterSeq: 10000n, snapshotSeq: 10500n },
      signal(),
    );
    expect(newer.snapshotSeq).toBe(10500n);
    expect(newer.events).toHaveLength(500);
    expect(newer.hasNewer).toBe(false);
    const older = await s.page(
      { beforeSeq: 100n, snapshotSeq: 10000n },
      signal(),
    );
    expect(older.events[0].seq).toBe(1n);
    expect(older.events.at(-1)?.seq).toBe(99n);
    expect(older.hasOlder).toBe(false);
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
    expect(s.mode).toBe("unknown");
    expect(history).not.toHaveBeenCalled();
  });
});
