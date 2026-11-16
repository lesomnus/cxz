import { expect, it } from "vitest";
import { mergeLogHead } from "./use-log-feed";
import { logEntry, type LogPage } from "#src/shared/logs/log-entry.ts";
const entry = (id: string) => ({
  ...logEntry(
    { level: "info", category: "client", message: id },
    "test",
    "test",
  ),
  id,
});
const page = (ids: string[], total: number, next?: string): LogPage => ({
  entries: ids.map(entry),
  total,
  next,
  bytes: 0,
  capacity: 10000,
  storage: "memory",
});
it("preserves contiguous loaded history while merging an overlapping live head", () => {
  const merged = mergeLogHead(
    page(["c", "b", "a"], 5, "a"),
    page(["e", "d", "c"], 7, "c"),
  );
  expect(merged.entries.map((entry) => entry.id)).toEqual([
    "e",
    "d",
    "c",
    "b",
    "a",
  ]);
  expect(merged.next).toBe("a");
});
it("starts at the new head after a burst with no overlap rather than joining across missing records", () => {
  const head = page(["z", "y", "x"], 50, "x");
  expect(mergeLogHead(page(["c", "b", "a"], 5, "a"), head)).toBe(head);
});
it("clears retained rows after deletion and trims history pruned by storage rotation", () => {
  expect(mergeLogHead(page(["c", "b", "a"], 5), page([], 0)).entries).toEqual(
    [],
  );
  const merged = mergeLogHead(
    page(["c", "b", "a"], 5, "a"),
    page(["d", "c"], 3, "c"),
  );
  expect(merged.entries.map((entry) => entry.id)).toEqual(["d", "c", "b"]);
  expect(merged.next).toBeUndefined();
});
