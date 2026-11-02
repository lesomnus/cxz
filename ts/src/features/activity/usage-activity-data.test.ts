import { create } from "@bufbuild/protobuf";
import { expect, it } from "vitest";
import { ConversationStatsReplySchema } from "#gen/cxz/session_svc_pb";
import {
  usageActivityDays,
  usageActivityCells,
  usageActivityRange,
  usageActivityWeeks,
  usageDateAge,
} from "./usage-activity-data";

it("fills sparse UTC dates in order and excludes the end at midnight", () => {
  const days = usageActivityDays(
    create(ConversationStatsReplySchema, {
      fromMs: BigInt(Date.parse("2026-10-30T20:00:00-04:00")),
      toMs: BigInt(Date.parse("2026-11-03T00:00:00Z")),
      days: [
        { date: "2026-11-02", values: { finishedTurns: 4n } },
        { date: "2026-10-31", values: { finishedTurns: 1n } },
        { date: "2026-11-03", values: { finishedTurns: 999n } },
      ],
    }),
  );
  expect(days.map(({ date }) => date)).toEqual([
    "2026-10-31",
    "2026-11-01",
    "2026-11-02",
  ]);
  expect(days[1].values).toBeUndefined();
  expect(days.map(({ level }) => level)).toEqual([1, 0, 4]);
});

it("includes both partial boundary days and renders empty ranges without activity", () => {
  const days = usageActivityDays(
    create(ConversationStatsReplySchema, {
      fromMs: BigInt(Date.parse("2026-10-31T23:59:00Z")),
      toMs: BigInt(Date.parse("2026-11-01T00:01:00Z")),
    }),
  );
  expect(days.map(({ date, level }) => [date, level])).toEqual([
    ["2026-10-31", 0],
    ["2026-11-01", 0],
  ]);
  expect(usageActivityDays(create(ConversationStatsReplySchema))).toEqual([]);
});

it("keeps small activity visible next to uint64-scale turn counts", () => {
  const days = usageActivityDays(
    create(ConversationStatsReplySchema, {
      fromMs: BigInt(Date.parse("2026-10-31T00:00:00Z")),
      toMs: BigInt(Date.parse("2026-11-03T00:00:00Z")),
      days: [
        { date: "2026-10-31", values: { finishedTurns: 1n } },
        {
          date: "2026-11-01",
          values: { finishedTurns: 18446744073709551615n },
        },
      ],
    }),
  );
  expect(days.map(({ level }) => level)).toEqual([1, 4, 0]);
});

it("fits whole weeks to the available width without exceeding the API range", () => {
  expect(usageActivityWeeks(332, 12, 4)).toBe(21);
  expect(usageActivityWeeks(316, 12, 4)).toBe(20);
  expect(usageActivityWeeks(0, 12, 4)).toBe(1);
  expect(usageActivityWeeks(2000, 12, 4)).toBe(52);
  const to = BigInt(Date.parse("2026-11-01T18:00:00Z"));
  const range = usageActivityRange(21, to);
  expect(new Date(Number(range.fromMs)).getUTCDay()).toBe(0);
  expect(
    usageActivityDays(create(ConversationStatsReplySchema, range)),
  ).toHaveLength(141);
  const maximum = usageActivityRange(52, to);
  expect(Number(maximum.toMs - maximum.fromMs)).toBeLessThanOrEqual(
    366 * 86400000,
  );
});

it("treats an exact midnight end as exclusive and humanizes UTC dates", () => {
  const toMs = BigInt(Date.parse("2026-11-01T00:00:00Z"));
  const days = usageActivityDays(
    create(ConversationStatsReplySchema, usageActivityRange(2, toMs)),
  );
  expect(days.at(-1)?.date).toBe("2026-10-31");
  expect(days).toHaveLength(14);
  const now = Date.parse("2026-11-01T00:01:00Z");
  expect(usageDateAge("2026-10-24", now, "en").label).toBe("8 days ago");
  expect(usageDateAge("2026-10-24", now, "en").days).toBe(8);
  expect(usageDateAge("2026-10-31", now, "en").label).toBe("yesterday");
  expect(usageDateAge("2026-11-01", now, "en").label).toBe("today");
  expect(
    usageDateAge("2026-11-01", now, "en").parts.some(
      (part) => part.type === "integer",
    ),
  ).toBe(false);
});

it("pads the final week across a year boundary without inventing activity", () => {
  const cells = usageActivityCells(
    usageActivityDays(
      create(ConversationStatsReplySchema, {
        fromMs: BigInt(Date.parse("2026-12-27T00:00:00Z")),
        toMs: BigInt(Date.parse("2027-01-04T12:00:00Z")),
      }),
    ),
  );
  expect(cells.filter((cell) => cell.day)).toHaveLength(9);
  expect(cells[0].date).toBe("2026-12-27");
  expect(cells.filter((cell) => cell.future).map((cell) => cell.date)).toEqual([
    "2027-01-05",
    "2027-01-06",
    "2027-01-07",
    "2027-01-08",
    "2027-01-09",
  ]);
  expect(cells.filter((cell) => cell.future).every((cell) => !cell.day)).toBe(
    true,
  );
  expect(usageActivityCells([])).toEqual([]);
});
