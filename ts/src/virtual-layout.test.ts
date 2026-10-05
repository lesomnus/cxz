import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { SessionEventSchema } from "../gen/cxz/session_pb";
import {
  messageLayout,
  messageMap,
  ROW_UNITS,
  rowAt,
  visibleRows,
} from "./virtual-layout";

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

it("maps variable pixel heights to stable message positions and back monotonically", () => {
  const layout = messageLayout(
    events.slice(0, 4),
    new Map([
      ["1", 300],
      ["2", 50],
      ["3", 500],
    ]),
  );
  const map = messageMap(
    layout.rows,
    12,
    new Map([["3", { offset: 120, fraction: 0.4 }]]),
  );
  let last = -Infinity;
  for (let pixel = 0; pixel < layout.total + 24; pixel += 3) {
    const logical = map.toLogical(pixel);
    expect(logical).toBeGreaterThan(last);
    expect(map.toNative(logical)).toBeCloseTo(pixel, 8);
    last = logical;
  }
  expect(map.toLogical(12 + layout.rows[2].top + 120)).toBe(
    12 + 2.4 * ROW_UNITS,
  );
});

it("keeps prompt positions and the reading fraction fixed when its row height changes", () => {
  const old = messageLayout(
    events.slice(0, 4),
    new Map([
      ["1", 100],
      ["2", 100],
    ]),
  );
  const next = messageLayout(
    events.slice(0, 4),
    new Map([
      ["1", 1000],
      ["2", 280],
    ]),
  );
  const before = messageMap(old.rows, 12, new Map());
  const after = messageMap(
    next.rows,
    12,
    new Map([["2", { offset: 40, fraction: 0.4 }]]),
  );
  expect(after.markers).toEqual(before.markers);
  expect(after.toLogical(12 + next.rows[1].top + 40)).toBe(
    before.toLogical(12 + old.rows[1].top + 40),
  );
});
