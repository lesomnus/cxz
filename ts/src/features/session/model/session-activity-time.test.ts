import { create } from "@bufbuild/protobuf";
import { TimestampSchema } from "@bufbuild/protobuf/wkt";
import { expect, it } from "vitest";
import { SessionSchema, SessionStatusSchema } from "#gen/cxz/session_pb";
import {
  sortSessionsByActivity,
  type ActivityCache,
} from "./session-activity-time";

const session = (runtimeId: string, createdSeconds: bigint) =>
  create(SessionSchema, {
    runtimeId,
    dateCreated: { seconds: createdSeconds },
  });

it("sorts by conversation time without prioritizing lifecycle state or metadata edits", () => {
  const old = session("old", 1n),
    latest = session("latest", 2n);
  old.status = create(SessionStatusSchema, { state: "working" });
  old.dateUpdated = create(TimestampSchema, { seconds: 999n });
  const activity: ActivityCache = new Map([
    [old.runtimeId, { version: "", time: 3000 }],
    [latest.runtimeId, { version: "", time: 4000 }],
  ]);
  const input = [old, latest];
  expect(sortSessionsByActivity(input, activity)).toEqual([latest, old]);
  expect(input).toEqual([old, latest]);
});

it("falls back to creation time for empty or unavailable conversations", () => {
  const used = session("used", 1n),
    empty = session("empty", 5n),
    missing = session("missing", 2n);
  const activity: ActivityCache = new Map([
    [used.runtimeId, { version: "", time: 4000 }],
    [empty.runtimeId, { version: "" }],
  ]);
  expect(sortSessionsByActivity([used, missing, empty], activity)).toEqual([
    empty,
    used,
    missing,
  ]);
});

it("breaks equal activity times by creation time and stable identity", () => {
  const a = session("a", 2n),
    b = session("b", 2n),
    older = session("older", 1n);
  const activity: ActivityCache = new Map(
    [a, b, older].map((s) => [s.runtimeId, { version: "", time: 3000 }]),
  );
  expect(sortSessionsByActivity([older, b, a], activity)).toEqual([
    a,
    b,
    older,
  ]);
  expect(sortSessionsByActivity([a, older, b], activity)).toEqual([
    a,
    b,
    older,
  ]);
});
