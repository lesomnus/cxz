import { useProject } from "#src/shared/api/use-project.ts";
import {
  projectId as projectUUID,
  projectRef,
} from "#src/shared/api/project-id.ts";
import { useRef, useState } from "react";
import { Id } from "@lesomnus/payday/pdid";
import { Button, PageToolbar } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Task } from "#gen/cxz/task_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ProjectPicker } from "#src/shared/components/project-picker.tsx";
import { useProjectTasks } from "./task-catalog";
import { TaskBoard } from "./task-board";
import { TaskDialog } from "./task-dialog";
import { taskKey } from "./task-model";
import styles from "./tasks.module.css";
import {
  matchesResource,
  resourceHandle,
} from "#src/shared/navigation/resource-route.ts";

export type TaskSelection = {
  project?: string;
  task?: string;
  assignee?: string;
  run?: string;
};
export function TasksView({
  c,
  projects,
  sessions,
  projectsLoading = false,
  sessionsLoading = false,
  selection,
  select,
}: {
  c: Connection;
  projects: Project[];
  sessions: Session[];
  projectsLoading?: boolean;
  sessionsLoading?: boolean;
  selection: TaskSelection;
  select: (value: TaskSelection) => void;
}) {
  const resolved = useProject(c, projects, selection.project);
  const needsProject =
    !!selection.task && !selection.project && projects.length > 1;
  const project = needsProject ? undefined : resolved;
  const projectId = projectUUID(project) ?? "";
  const state = useProjectTasks(c, projectId);
  const remembered = useRef<
    { handle?: string; project: string; id: string } | undefined
  >(undefined);
  const selectedTask =
    state.tasks.find(
      (task) =>
        !!selection.task &&
        (matchesResource(task, selection.task) ||
          taskKey(task) === selection.task),
    ) ??
    (remembered.current?.handle === selection.task &&
    remembered.current?.project === projectId
      ? state.tasks.find((task) => taskKey(task) === remembered.current?.id)
      : undefined);
  if (selectedTask)
    remembered.current = {
      handle: selection.task,
      project: projectId,
      id: taskKey(selectedTask),
    };
  const selectedTaskId = selectedTask
    ? new Id(selectedTask.id).toString()
    : selection.task;
  const [adding, setAdding] = useState(false);
  const open = (task: Task) =>
    select({
      ...selection,
      project: project && resourceHandle(project),
      task: resourceHandle(task),
    });
  const close = () => {
    setAdding(false);
    if (selection.task)
      select({
        ...selection,
        project: project && resourceHandle(project),
        task: undefined,
      });
  };
  const assigned = sessions.find(
    (session) => session.runtimeId === selection.assignee,
  );
  return (
    <>
      <main className={styles.page} aria-label="Project tasks">
        <PageToolbar
          aria-label="Task toolbar"
          actions={
            <Button
              type="button"
              variant="toolbar"
              aria-label="Add task"
              title="Add task"
              disabled={!project}
              onClick={() => setAdding(true)}
            >
              <svg
                width="18"
                height="18"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                aria-hidden="true"
              >
                <path d="M12 5v14M5 12h14" />
              </svg>
            </Button>
          }
        >
          <ProjectPicker
            projects={projects}
            value={projectId}
            loading={projectsLoading}
            choose={(project) => {
              setAdding(false);
              select({
                project: resourceHandle(
                  projects.find((p) => projectUUID(p) === project)!,
                ),
                ...(needsProject ? { task: selection.task } : {}),
              });
            }}
          />
        </PageToolbar>
        <div className={styles.body}>
          {project ? (
            <>
              {!!selection.assignee && (
                <Button
                  type="button"
                  className={styles.filter}
                  onClick={() => select({ project: resourceHandle(project) })}
                  aria-label="Show all project tasks"
                >
                  {assigned?.alias ?? "Assigned tasks"}
                  {selection.run ? " · This run" : ""} ×
                </Button>
              )}
              {state.loading && !state.tasks.length && (
                <p role="status" className={styles.message}>
                  Loading tasks…
                </p>
              )}
              {state.error && (
                <div className={styles.message}>
                  <p role="alert">{state.error}</p>
                  <span>
                    Tasks are stored in the project runtime. Start the project
                    if it is stopped.
                  </span>
                  <Button type="button" onClick={state.refresh}>
                    Retry
                  </Button>
                </div>
              )}
              {(!state.loading || !!state.tasks.length) &&
                (!state.error || !!state.tasks.length) && (
                  <TaskBoard
                    key={projectId}
                    tasks={state.tasks}
                    sessions={sessions}
                    sessionsLoading={sessionsLoading}
                    assignee={selection.assignee}
                    run={selection.run}
                    open={open}
                  />
                )}
            </>
          ) : (
            <p className={styles.message}>
              {projectsLoading
                ? "Loading projects…"
                : needsProject
                  ? "Choose a project to open this task."
                  : selection.project
                    ? "Project not found. Choose another project."
                    : "Add a project to start tracking shared work."}
            </p>
          )}
        </div>
      </main>
      {project && (adding || selection.task) && (
        <TaskDialog
          key={`${projectId}:${adding ? "add" : selectedTaskId}`}
          c={c}
          project={project}
          selected={adding ? undefined : selectedTaskId}
          latest={selectedTask}
          sessions={sessions}
          close={close}
          saved={() => close()}
        />
      )}
    </>
  );
}
