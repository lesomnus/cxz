import { projectId, projectRef } from "#src/shared/api/project-id.ts";
import type { Task } from "#gen/cxz/task_pb";
import { TaskStatus } from "#gen/cxz/task_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Project } from "#gen/cxz/project_pb";
import { bytes, key } from "@lesomnus/payday/store";

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
    : { case: "alias" as const, value },
  project: projectRef(projectId(project)),
});
export const taskKey = (task: Task) => key(task.id);
export const taskOwner = (task: Task) => ({
  session: { key: { case: "runtimeId" as const, value: task.assignee } },
  runId: task.assigneeRun,
});
export const taskTime = (task: Task) =>
  Number(task.dateUpdated?.seconds ?? 0n) * 1000 +
  (task.dateUpdated?.nanos ?? 0) / 1000000;
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

export function boardTasks(
  tasks: Task[],
  assignee = "",
  run = "",
  allDone = false,
) {
  const visible = tasks.filter(
    (task) =>
      (!assignee || task.assignee === assignee) &&
      (!run || task.assigneeRun === run),
  );
  return taskStatuses.map((column) => {
    const items = visible
      .filter((task) => task.status === column.value)
      .sort((a, b) =>
        column.value === TaskStatus.DONE
          ? taskTime(b) - taskTime(a)
          : a.priority - b.priority || taskTime(b) - taskTime(a),
      );
    return {
      ...column,
      total: items.length,
      items:
        column.value === TaskStatus.DONE && !allDone
          ? items.slice(0, 12)
          : items,
    };
  });
}

export const sessionInProject = (session: Session, project: Project) =>
  !!session.project && key(session.project.id) === key(project.id);
