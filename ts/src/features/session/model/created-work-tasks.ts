import type { Task } from "#gen/cxz/task_pb";
import type { WorkGroup } from "./work-groups";

type TaskRange = Pick<WorkGroup, "id" | "anchor" | "fromMs" | "toMs">;
const time = (task: Task) =>
  task.dateCreated &&
  task.dateCreated.seconds * 1_000_000_000n + BigInt(task.dateCreated.nanos);

/** Index once per catalog update instead of scanning every Task for each row. */
export function createdTasksByWorkGroup(
  tasks: readonly Task[],
  session: string,
  groups: Iterable<TaskRange>,
) {
  const result = new Map<string, Task[]>();
  if (!session) return result;
  const runs = new Map<string, TaskRange[]>();
  for (const group of groups) {
    if (!group.anchor.runId || !group.fromMs) continue;
    let ranges = runs.get(group.anchor.runId);
    if (!ranges) runs.set(group.anchor.runId, (ranges = []));
    ranges.push(group);
  }
  for (const ranges of runs.values())
    ranges.sort((a, b) =>
      a.fromMs! < b.fromMs! ? -1 : a.fromMs! > b.fromMs! ? 1 : 0,
    );
  const ordered = tasks
    .filter(
      (task) => task.sourceSession === session && time(task) !== undefined,
    )
    .sort((a, b) => {
      const delta = time(a)! - time(b)!;
      return delta < 0n ? -1 : delta > 0n ? 1 : a.alias.localeCompare(b.alias);
    });
  for (const task of ordered) {
    const ranges = runs.get(task.sourceRun);
    if (!ranges) continue;
    const created = time(task)!;
    let low = 0,
      high = ranges.length;
    while (low < high) {
      const middle = (low + high) >>> 1;
      if (ranges[middle].fromMs! * 1_000_000n <= created) low = middle + 1;
      else high = middle;
    }
    const group = ranges[low - 1];
    if (
      !group ||
      (group.toMs !== undefined && created >= group.toMs * 1_000_000n)
    )
      continue;
    let matches = result.get(group.id);
    if (!matches) result.set(group.id, (matches = []));
    matches.push(task);
  }
  return result;
}

/** Creation provenance is immutable; claiming or editing a Task does not move it. */
export function createdWorkTasks(
  tasks: readonly Task[],
  session: string,
  group: Pick<WorkGroup, "anchor" | "fromMs" | "toMs">,
) {
  return (
    createdTasksByWorkGroup(tasks, session, [{ ...group, id: "range" }]).get(
      "range",
    ) ?? []
  );
}
