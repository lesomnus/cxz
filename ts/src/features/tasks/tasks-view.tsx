import { useRef, useState } from "react";
import { Button, PanelItem } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Task } from "#gen/cxz/task_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ResourcePanel } from "#src/shared/components/resource-panel.tsx";
import { ResourceValue } from "#src/shared/components/resource-value.tsx";
import { useProjectTasks } from "./task-catalog";
import { TaskBoard } from "./task-board";
import { TaskDialog } from "./task-dialog";
import { taskKey } from "./task-model";
import styles from "./tasks.module.css";

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
  const project =
    projects.find((project) => project.runtimeId === selection.project) ??
    (!selection.project ? projects[0] : undefined);
  const projectId = project?.runtimeId ?? "";
  const state = useProjectTasks(c, projectId);
  const [adding, setAdding] = useState(false);
  const panel = useRef<HTMLElement>(null);
  const open = (task: Task) =>
    select({ ...selection, project: projectId, task: taskKey(task) });
  const close = () => {
    setAdding(false);
    if (selection.task)
      select({ ...selection, project: projectId, task: undefined });
  };
  const assigned = sessions.find(
    (session) => session.runtimeId === selection.assignee,
  );
  return (
    <>
      <ResourcePanel
        ref={panel}
        title="Tasks"
        aria-label="Task projects"
        actions={
          <Button
            type="button"
            aria-label="Add task"
            title="Add task"
            disabled={!project}
            onClick={() => setAdding(true)}
          >
            +
          </Button>
        }
      >
        {projects.map((item) => (
          <PanelItem
            key={item.runtimeId}
            aria-pressed={item.runtimeId === projectId}
            onClick={() => {
              setAdding(false);
              select({ project: item.runtimeId });
            }}
          >
            <span className={styles.project}>
              <strong>{item.name || item.alias}</strong>
              <small>{item.alias}</small>
            </span>
          </PanelItem>
        ))}
        {projectsLoading && !projects.length && (
          <p className={styles.quiet}>Loading projects…</p>
        )}
        {!projectsLoading && !projects.length && (
          <p className={styles.quiet}>No projects yet.</p>
        )}
      </ResourcePanel>
      <main className={styles.page}>
        {project ? (
          <>
            <header className={styles.heading}>
              <div className={styles.projectHeading}>
                <h1>{project.name || project.alias}</h1>
                <small>{project.workspace}</small>
              </div>
              <div className={styles.mobileProject}>
                <ResourceValue
                  label="Project"
                  value={projectId}
                  options={projects.map((item) => ({
                    value: item.runtimeId,
                    label: item.name || item.alias,
                  }))}
                  choose={(project) => {
                    setAdding(false);
                    select({ project });
                  }}
                />
              </div>
              {!!selection.assignee && (
                <Button
                  type="button"
                  onClick={() => select({ project: projectId })}
                  aria-label="Show all project tasks"
                >
                  {assigned?.alias ?? "Assigned tasks"}
                  {selection.run ? " · This run" : ""} ×
                </Button>
              )}
              <Button
                type="button"
                className={styles.mobileAdd}
                onClick={() => setAdding(true)}
              >
                Add task
              </Button>
            </header>
            {state.loading && !state.tasks.length && (
              <p role="status" className={styles.message}>
                Loading tasks…
              </p>
            )}
            {state.error && (
              <div className={styles.message}>
                <p role="alert">{state.error}</p>
                <span>
                  Tasks are stored in the project runtime. Start the project if
                  it is stopped.
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
              : selection.project
                ? "Project not found. Choose another project."
                : "Add a project to start tracking shared work."}
          </p>
        )}
      </main>
      {project && (adding || selection.task) && (
        <TaskDialog
          key={`${projectId}:${adding ? "add" : selection.task}`}
          c={c}
          project={project}
          selected={adding ? undefined : selection.task}
          latest={state.tasks.find(
            (task) =>
              taskKey(task) === selection.task || task.alias === selection.task,
          )}
          sessions={sessions}
          close={close}
          saved={() => close()}
        />
      )}
    </>
  );
}
