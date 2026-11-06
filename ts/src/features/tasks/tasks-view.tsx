import { ToolbarIconButton } from "#src/shared/components/toolbar-icon-button.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useProject } from "#src/shared/api/use-project.ts";
import { projectId as projectUUID } from "#src/shared/api/project-id.ts";
import { useCallback, useRef, useState } from "react";
import { useBlocker } from "@tanstack/react-router";
import { Id } from "@lesomnus/payday/pdid";
import { Button } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Task } from "#gen/cxz/task_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ProjectPicker } from "#src/shared/components/project-picker.tsx";
import { SessionPicker } from "#src/shared/components/session-picker.tsx";
import { useProjectTasks } from "./task-catalog";
import { TaskTable } from "./task-table";
import { TaskDialog } from "./task-dialog";
import { TaskDetailPage } from "./task-detail-page";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import { taskKey } from "./task-model";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import {
  ResourceLayout,
  ResourcePage,
  ResourceToolbar,
} from "#src/shared/components/resource-layout.tsx";
import pageStyles from "#src/shared/components/resource-page.module.css";
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
  select: (value: TaskSelection, options?: { replace?: boolean }) => void;
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
  const [busy, setBusy] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [editorRevision, setEditorRevision] = useState(0);
  const dirtyRef = useRef(false);
  const onDirtyChange = useCallback((value: boolean) => {
    dirtyRef.current = value;
    setDirty(value);
  }, []);
  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) =>
      !!selection.task &&
      dirtyRef.current &&
      (current.pathname !== next.pathname ||
        JSON.stringify(current.search) !== JSON.stringify(next.search)),
    enableBeforeUnload: dirty,
    withResolver: true,
  });
  const open = (task: Task) =>
    select(
      {
        ...selection,
        project: project && resourceHandle(project),
        task: resourceHandle(task),
      },
      { replace: true },
    );
  const close = () => {
    setAdding(false);
    if (selection.task)
      select({
        ...selection,
        project: project && resourceHandle(project),
        task: undefined,
      });
  };
  return (
    <>
      <ResourcePage aria-label="Project tasks">
        <ResourceToolbar
          aria-label="Task toolbar"
          actions={
            <ToolbarIconButton
              icon="add"
              label="Add task"
              disabled={!project}
              onClick={() => setAdding(true)}
            />
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
          <SessionPicker
            sessions={sessions}
            project={projectId}
            value={selection.assignee}
            loading={sessionsLoading}
            suffix={selection.run ? " · This run" : ""}
            getValue={(session) => session.runtimeId}
            choose={(assignee) =>
              select({
                ...selection,
                assignee: assignee || undefined,
                run: undefined,
              })
            }
          />
        </ResourceToolbar>
        {project && selection.task ? (
          <TaskDetailPage
            c={c}
            project={project}
            tasks={state.tasks}
            sessions={sessions}
            selected={selectedTaskId!}
            latest={selectedTask}
            assignee={selection.assignee}
            run={selection.run}
            open={open}
            close={close}
            saved={open}
            busy={busy}
            setBusy={setBusy}
            onDirtyChange={onDirtyChange}
            editorRevision={editorRevision}
          />
        ) : (
          <PanelScroll
            className={styles.body}
            contentClassName={pageStyles.contentPadding}
            horizontalScrollbar
            scrollbarLabel="Scroll tasks"
          >
            <ResourceLayout className={styles.content}>
              {project ? (
                <>
                  {state.loading && !state.tasks.length && (
                    <p role="status" className={styles.message}>
                      Loading tasks…
                    </p>
                  )}
                  {state.error && (
                    <div className={styles.message}>
                      <ErrorNotice as="div" error={state.error}>
                        <p>{state.error}</p>
                        <span>
                          Tasks are stored in the project runtime. Start the
                          project if it is stopped.
                        </span>
                      </ErrorNotice>
                      <Button type="button" onClick={state.refresh}>
                        Retry
                      </Button>
                    </div>
                  )}
                  {(!state.loading || !!state.tasks.length) &&
                    (!state.error || !!state.tasks.length) && (
                      <TaskTable
                        key={JSON.stringify([
                          projectId,
                          selection.assignee,
                          selection.run,
                        ])}
                        tasks={state.tasks}
                        sessions={sessions}
                        sessionsLoading={sessionsLoading}
                        project={project}
                        assignee={selection.assignee}
                        run={selection.run}
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
            </ResourceLayout>
          </PanelScroll>
        )}
      </ResourcePage>
      {project && adding && (
        <TaskDialog
          key={`${projectId}:add`}
          c={c}
          project={project}
          sessions={sessions}
          close={() => setAdding(false)}
          saved={() => setAdding(false)}
        />
      )}
      {blocker.status === "blocked" && (
        <ConfirmationDialog
          title="Discard unsaved changes"
          confirmLabel="Discard changes"
          close={() => blocker.reset?.()}
          failureMessage="Wait for the task request to finish before leaving."
          execute={() => {
            if (busy) return false;
            onDirtyChange(false);
            setEditorRevision((revision) => revision + 1);
            blocker.proceed?.();
            return true;
          }}
        >
          <p>
            This task has unsaved changes. Discard them and leave this task?
          </p>
        </ConfirmationDialog>
      )}
    </>
  );
}
