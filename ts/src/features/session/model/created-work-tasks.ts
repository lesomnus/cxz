import type { Task } from "#gen/cxz/task_pb";
import type { WorkGroup } from "./work-groups";

/** Creation provenance is immutable; claiming or editing a Task does not move it. */
export function createdWorkTasks(
  tasks: readonly Task[],
  session: string,
  group: Pick<WorkGroup, "anchor" | "fromMs" | "toMs">,
) {
  if (!session || !group.anchor.runId || !group.fromMs) return [];
  const from = group.fromMs * 1_000_000n;
  const to = group.toMs === undefined ? undefined : group.toMs * 1_000_000n;
  const time = (task: Task) =>
    task.dateCreated &&
    task.dateCreated.seconds * 1_000_000_000n + BigInt(task.dateCreated.nanos);
  return tasks
    .filter((task) => {
      const created = time(task);
      return (
        task.sourceSession === session &&
        task.sourceRun === group.anchor.runId &&
        created !== undefined &&
        created >= from &&
        (to === undefined || created < to)
      );
    })
    .sort((a, b) => {
      const delta = time(a)! - time(b)!;
      return delta < 0n ? -1 : delta > 0n ? 1 : a.alias.localeCompare(b.alias);
    });
}
