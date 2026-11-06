import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { sessionUUID } from "#src/shared/api/session-id.ts";
import { useResourceSelection } from "#src/shared/api/resource-selection.ts";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { TasksView } from "#src/features/tasks/tasks-view.tsx";
import {
  taskLocation,
  matchesResource,
  resourceHandle,
} from "#src/shared/navigation/resource-route.ts";

export function TasksPage(props: {
  c: Connection;
  projects: Project[];
  sessions: Session[];
  projectsLoading?: boolean;
  sessionsLoading?: boolean;
}) {
  const navigate = useNavigate();
  const { tasks } = useWorkspaceRoute();
  const stored = useResourceSelection(props.c);
  const project = tasks.project
    ? props.projects.find((p) => matchesResource(p, tasks.project!))
    : tasks.task
      ? props.projects.length === 1
        ? props.projects[0]
        : undefined
      : (props.projects.find((p) => projectId(p) === stored.project) ??
        props.projects[0]);
  const id = projectId(project);
  const selected = {
    ...tasks,
    project: tasks.project ?? (project && resourceHandle(project)),
    assignee:
      tasks.assignee ??
      (stored.project === id ? stored.assignee || undefined : undefined),
  };
  const routedSession = props.sessions.find(
    (session) =>
      session.project &&
      projectId(session.project) === id &&
      session.runtimeId === tasks.assignee,
  );
  const routedSessionId = sessionUUID(routedSession?.id);
  useEffect(() => {
    if (!id) return;
    if (tasks.assignee !== undefined) {
      const previous = props.c.selection.snapshot();
      props.c.selection.chooseSession(
        id,
        routedSessionId ||
          (previous.project === id && previous.assignee === tasks.assignee
            ? previous.session
            : ""),
        tasks.assignee,
      );
    } else props.c.selection.chooseProject(id);
    if (
      (!tasks.project && selected.project) ||
      (!tasks.assignee && selected.assignee)
    )
      void navigate({ ...taskLocation(selected), replace: true });
  }, [
    props.c,
    id,
    tasks.project,
    tasks.task,
    tasks.assignee,
    tasks.run,
    routedSessionId,
    selected.project,
    selected.assignee,
    navigate,
  ]);
  return (
    <TasksView
      {...props}
      selection={selected}
      select={(next, options) => {
        const target = props.projects.find(
          (p) => next.project && matchesResource(p, next.project),
        );
        if (target) {
          const targetId = projectId(target);
          const session = props.sessions.find(
            (s) =>
              s.project &&
              projectId(s.project) === targetId &&
              s.runtimeId === next.assignee,
          );
          const previous = props.c.selection.snapshot();
          props.c.selection.chooseSession(
            targetId,
            sessionUUID(session?.id) ||
              (previous.project === targetId &&
              previous.assignee === next.assignee
                ? previous.session
                : ""),
            next.assignee ?? "",
          );
        }
        void navigate({ ...taskLocation(next), ...options });
      }}
    />
  );
}
