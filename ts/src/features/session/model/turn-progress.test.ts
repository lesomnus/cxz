import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import {
  SessionEventSchema,
  SessionStatusSchema,
} from "../../../../gen/cxz/session_pb";
import { advanceTurn, snapshotTurn } from "./turn-progress";

const event = (seq: number, kind: string, text = "", runId = "run") =>
  create(SessionEventSchema, {
    seq: BigInt(seq),
    timeMs: BigInt(seq * 1000),
    kind,
    text,
    runId,
  });
const status = (state: string, runId = "run") =>
  create(SessionStatusSchema, { state, runId });

describe("current turn timing", () => {
  it("preserves the first input across steering and permission round trips", () => {
    const initial = advanceTurn(
      { runId: "run", active: false },
      event(1, "input"),
    );
    const waiting = advanceTurn(initial, event(3, "state", "waiting_input"));
    const resumed = advanceTurn(waiting, event(4, "state", "working"));
    expect(advanceTurn(resumed, event(5, "input"))).toEqual(initial);
    expect(advanceTurn(resumed, event(6, "usage"))).toBe(resumed);
  });
  it("resets completion and gives the next turn a new confirmation identity", () => {
    const initial = advanceTurn(
      { runId: "run", active: false },
      event(1, "input"),
    );
    const done = advanceTurn(initial, event(3, "turn_end", "interrupted"));
    expect(done).toEqual({ runId: "run", active: false });
    const next = advanceTurn(done, event(5, "input"));
    expect(next.startedAt).toBe(5000);
    expect(next.key).not.toBe(initial.key);
    expect(advanceTurn(next, event(6, "state", "idle")).active).toBe(false);
  });
  it("ignores old runs and recovers the current start from the latest history", () => {
    const turn = snapshotTurn(status("working"), [
      event(1, "input"),
      event(2, "turn_end", "completed"),
      event(3, "input"),
      event(4, "input", "", "old"),
    ]);
    expect(turn.startedAt).toBe(3000);
    expect(advanceTurn(turn, event(5, "state", "idle", "old"))).toBe(turn);
    expect(snapshotTurn(status("working"), [], turn)).toBe(turn);
    expect(snapshotTurn(status("idle"), [], turn).active).toBe(false);
  });
  it("accepts sandbox state names and retains timing while waiting for a question", () => {
    const turn = snapshotTurn(status("running"), [event(1, "input")]);
    expect(advanceTurn(turn, event(2, "state", "waiting"))).toBe(turn);
    expect(snapshotTurn(status("waiting"), [], turn)).toBe(turn);
  });
  it("follows a resumed run in the ordered live stream without carrying its old clock", () => {
    const old = snapshotTurn(status("working"), [event(1, "input")]);
    const resumed = advanceTurn(
      old,
      event(2, "state", "starting", "new"),
      true,
    );
    expect(resumed).toEqual({ runId: "new", active: false });
    const next = advanceTurn(resumed, event(3, "input", "", "new"), true);
    expect(next.startedAt).toBe(3000);
    expect(next.key).not.toBe(old.key);
  });
});
