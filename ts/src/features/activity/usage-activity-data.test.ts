import { create } from "@bufbuild/protobuf";
import { expect, it } from "vitest";
import { ConversationStatsReplySchema } from "#gen/cxz/session_svc_pb";
import { usageActivityDays } from "./usage-activity-data";

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
