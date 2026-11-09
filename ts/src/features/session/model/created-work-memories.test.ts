import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { MemorySchema } from "#gen/cxz/memory_pb";
import { createdMemoriesByWorkGroup } from "./created-work-memories";

const memory = (alias: string, ms: number) =>
  create(MemorySchema, {
    alias,
    dateCreated: {
      seconds: BigInt(Math.floor(ms / 1000)),
      nanos: (ms % 1000) * 1_000_000,
    },
  });
const ranges = [
  { id: "first", fromMs: 1000n, toMs: 2000n },
  { id: "second", fromMs: 2000n, toMs: 3000n },
  { id: "live", fromMs: 4000n },
];
describe("Memories created between replies", () => {
  it("uses creation time and half-open nanosecond-accurate ranges, including live ranges", () => {
    const edited = memory("edited", 999);
    edited.dateUpdated = memory("", 2500).dateCreated;
    const before = memory("before-end", 1999);
    before.dateCreated!.nanos += 999999;
    const deleted = memory("deleted", 1500);
    deleted.dateDeleted = memory("", 2500).dateCreated;
    const indexed = createdMemoriesByWorkGroup(
      [
        memory("live", 5000),
        memory("boundary", 2000),
        before,
        memory("start", 1000),
        edited,
        memory("gap", 3500),
        deleted,
        create(MemorySchema, { alias: "missing-time" }),
      ],
      [...ranges].reverse(),
    );
    expect(indexed.get("first")?.map((m) => m.alias)).toEqual([
      "start",
      "before-end",
    ]);
    expect(indexed.get("second")?.map((m) => m.alias)).toEqual(["boundary"]);
    expect(indexed.get("live")?.map((m) => m.alias)).toEqual(["live"]);
    expect([...indexed.values()].flat()).toHaveLength(4);
  });
  it("accepts an epoch-zero lower bound and ignores missing range starts", () => {
    const indexed = createdMemoriesByWorkGroup(
      [memory("epoch", 0)],
      [{ id: "unknown" }, { id: "epoch", fromMs: 0n, toMs: 1n }],
    );
    expect(indexed.get("epoch")?.map((m) => m.alias)).toEqual(["epoch"]);
  });
});
