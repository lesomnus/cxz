import { expect, it } from "vitest";
import { backgroundTasks } from "./background-report";
it("reads only the current run's reduced task snapshot", () => {
  const bytes = new TextEncoder().encode(
    JSON.stringify({
      old: { Tasks: { task: { task_id: "old" } } },
      current: {
        Snapshot: true,
        Tasks: {
          task: {
            task_id: "task",
            active: true,
            description: "Build",
            status: "working",
          },
          malformed: 2,
        },
      },
    }),
  );
  expect(backgroundTasks(bytes, "current")).toEqual([
    { task_id: "task", active: true, description: "Build", status: "working" },
  ]);
  expect(backgroundTasks(bytes, "missing")).toEqual([]);
});
