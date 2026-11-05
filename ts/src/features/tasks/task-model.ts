import { projectId, projectRef } from "#src/shared/api/project-id.ts";
import type { Task } from "#gen/cxz/task_pb";
import { TaskStatus } from "#gen/cxz/task_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Project } from "#gen/cxz/project_pb";
import { bytes, key } from "@lesomnus/payday/store";
import { resourceRef } from "#src/shared/navigation/resource-route.ts";

export const taskStatuses = [
  { value: TaskStatus.TODO, label: "Todo" },
  { value: TaskStatus.IN_PROGRESS, label: "In progress" },
  { value: TaskStatus.WAITING, label: "Waiting" },
  { value: TaskStatus.DONE, label: "Done" },
] as const;
export const taskStatusName = (status: TaskStatus) =>
  taskStatuses.find((item) => item.value === status)?.label ??
  (status === TaskStatus.CANCELED ? "Canceled" : "Unknown");
export const taskRef = (project: Project, task: Task) => ({
  key: { case: "id" as const, value: task.id },
  project: projectRef(projectId(project)),
});
export const taskLookup = (project: Project, value: string) => ({
  key: /^[0-9a-f]{32}$/i.test(value)
    ? { case: "id" as const, value: bytes(value) }
    : resourceRef(value).key,
  project: projectRef(projectId(project)),
});
export const taskKey = (task: Task) => key(task.id);
export const taskOwner = (task: Task) => ({
  session: { key: { case: "runtimeId" as const, value: task.assignee } },
  runId: task.assigneeRun,
});
export const activeTask = (task: Task) =>
  task.status === TaskStatus.IN_PROGRESS || task.status === TaskStatus.WAITING;

export function ownerIssue(task: Task, sessions: Session[], loading = false) {
  if (!task.assignee || !activeTask(task)) return "";
  const owner = sessions.find((session) => session.runtimeId === task.assignee);
  if (!owner) return loading ? "" : "Session unavailable";
  if (owner.status?.runId !== task.assigneeRun) return "Previous run";
  if (!["idle", "working", "waiting_input"].includes(owner.status?.state ?? ""))
    return "Session stopped";
  return "";
}

export function tableTasks(tasks: Task[], assignee = "", run = "") {
  return tasks
    .filter(
      (task) =>
        (!assignee || task.assignee === assignee) &&
        (!run || task.assigneeRun === run),
    )
    .sort(
      (a, b) =>
        Number(a.status === TaskStatus.DONE) -
          Number(b.status === TaskStatus.DONE) ||
        Number(
          (b.dateCreated?.seconds ?? 0n) - (a.dateCreated?.seconds ?? 0n),
        ) ||
        (b.dateCreated?.nanos ?? 0) - (a.dateCreated?.nanos ?? 0) ||
        taskKey(b).localeCompare(taskKey(a)),
    );
}

export const taskHistoryPageSize = 50;

export function taskTablePage(
  tasks: readonly Task[],
  now: number,
  olderDone = 0,
) {
  const yesterday = new Date(now);
  yesterday.setHours(0, 0, 0, 0);
  yesterday.setDate(yesterday.getDate() - 1);
  const cutoff = yesterday.getTime();
  let remaining = olderDone;
  let hidden = 0;
  const rows = tasks.filter((task) => {
    if (task.status !== TaskStatus.DONE) return true;
    const updated = task.dateUpdated ?? task.dateCreated;
    if (
      updated &&
      Number(updated.seconds) * 1000 + updated.nanos / 1e6 >= cutoff
    )
      return true;
    if (remaining > 0) {
      remaining--;
      return true;
    }
    hidden++;
    return false;
  });
  return { rows, hidden };
}

export const sessionInProject = (session: Session, project: Project) =>
  !!session.project && key(session.project.id) === key(project.id);
