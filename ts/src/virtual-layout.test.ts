import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { SessionEventSchema } from "../gen/cxz/session_pb";
import { messageLayout, rowAt, visibleRows } from "./virtual-layout";

const events = Array.from({ length: 2000 }, (_, i) =>
  create(SessionEventSchema, {
    seq: BigInt(i + 1),
    kind: i % 10 === 0 ? "input" : "assistant",
  }),
);
describe("variable height message window", () => {
  it("mounts the viewport plus six messages on either side in a large history", () => {
    const { rows } = messageLayout(events, new Map());
    const window = visibleRows(rows, rows[1000].top, 700);
    expect(window.start).toBe(994);
    expect(window.end - window.start).toBeLessThan(25);
    expect(window.end).toBe(rowAt(rows, rows[1000].top + 700) + 7);
  });
  it("uses measured heights for spacers, prompt markers and the visible anchor", () => {
    const original = messageLayout(events, new Map());
    const measured = messageLayout(
      events,
      new Map([
        ["1", 1500],
        ["2", 40],
      ]),
    );
    const shift = 1500 - original.rows[0].height + 40 - original.rows[1].height;
    expect(measured.rows[1000].top - original.rows[1000].top).toBe(shift);
    expect(rowAt(measured.rows, measured.rows[1000].top + 10)).toBe(1000);
    expect(measured.rows[1000].prompt).toBe(true);
    expect(rowAt(measured.rows, 1200)).toBe(0);
  });
  it("keeps uint64 IDs and bounds windows at either end and empty histories", () => {
    const { rows, total } = messageLayout(events, new Map());
    expect(visibleRows(rows, 0, 700).start).toBe(0);
    expect(visibleRows(rows, total, 700).end).toBe(2000);
    expect(visibleRows([], 0, 700)).toEqual({ start: 0, end: 0 });
    const large = create(SessionEventSchema, { seq: 9007199254740993n });
    expect(messageLayout([large], new Map()).rows[0].id).toBe(
      "9007199254740993",
    );
  });
});
