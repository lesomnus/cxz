import { expect, it } from "vitest";
import {
  backgroundStateTasks,
  visibleBackgroundTasks,
  type BackgroundTask,
} from "./background-tasks";

const task = (
  task_id: string,
  active: boolean,
  updated_seq?: string,
): BackgroundTask => ({
  task_id,
  active,
  updated_seq,
  description: task_id,
  status: active ? "working" : "completed",
  summary: "",
  output_file: "",
});
it("puts every active job first and keeps only three recent inactive jobs without mutating storage", () => {
  const tasks = [
    task("old", false, "1"),
    task("latest", false, "99"),
    task("active-old", true, "2"),
    task("third", false, "80"),
    task("second", false, "90"),
    task("active-new", true, "3"),
    task("fourth", false, "70"),
  ];
  const original = [...tasks];
  expect(visibleBackgroundTasks(tasks).map((t) => t.task_id)).toEqual([
    "active-new",
    "active-old",
    "latest",
    "second",
    "third",
  ]);
  expect(tasks).toEqual(original);
  expect(
    backgroundStateTasks({
      Tasks: Object.fromEntries(tasks.map((task) => [task.task_id, task])),
    }),
  ).toHaveLength(7);
});
it("compares full sequence precision, with timestamps and a stable fallback for old records", () => {
  const a = task("a", false, "9007199254740993");
  const b = task("b", false, "9007199254740994");
  expect(visibleBackgroundTasks([a, b]).map((t) => t.task_id)).toEqual([
    "b",
    "a",
  ]);
  const oldA = task("a", false),
    oldB = task("b", false);
  expect(visibleBackgroundTasks([oldB, oldA]).map((t) => t.task_id)).toEqual([
    "a",
    "b",
  ]);
  oldB.updated_at_ms = 42;
  expect(visibleBackgroundTasks([oldA, oldB]).map((t) => t.task_id)).toEqual([
    "b",
    "a",
  ]);
});
it("does not cap active jobs and handles malformed optional ordering metadata", () => {
  const tasks = Array.from({ length: 5 }, (_, i) =>
    task(`active-${i}`, true, i === 0 ? "invalid" : undefined),
  );
  expect(visibleBackgroundTasks(tasks)).toHaveLength(5);
  expect(visibleBackgroundTasks([])).toEqual([]);
});
