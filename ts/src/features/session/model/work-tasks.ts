import { useMemo, useSyncExternalStore } from "react";
import type { Project } from "#gen/cxz/project_pb";
import type { Task } from "#gen/cxz/task_pb";
import type { TaskCatalog } from "#src/features/tasks/task-catalog.ts";
import { projectId } from "#src/shared/api/project-id.ts";

export type WorkTaskSource = {
  project: Project;
  session: string;
  taskCatalog: TaskCatalog;
};
export type WorkTaskState = {
  tasks: Task[];
  loading: boolean;
  error: string;
  refresh: () => void;
};
const empty = { tasks: [] as Task[], loading: false, error: "" };
const emptySnapshot = () => empty;
const noop = () => {};
const emptySubscribe = () => noop;

/** One shared project subscription supplies both counters and open dialogs. */
export function useWorkTasks(source?: WorkTaskSource): WorkTaskState {
  const project = projectId(source?.project);
  const state = useMemo(
    () => source?.taskCatalog.forProject(project),
    [source?.taskCatalog, project],
  );
  const snapshot = useSyncExternalStore(
    state?.subscribe ?? emptySubscribe,
    state?.snapshot ?? emptySnapshot,
  );
  return useMemo(
    () => ({ ...snapshot, refresh: state?.refresh ?? noop }),
    [snapshot, state],
  );
}
