import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { TasksView } from "#src/features/tasks/tasks-view.tsx";
import { taskLocation } from "#src/shared/navigation/resource-route.ts";

export function TasksPage(props: {
  c: Connection;
  projects: Project[];
  sessions: Session[];
  projectsLoading?: boolean;
  sessionsLoading?: boolean;
}) {
  const navigate = useNavigate();
  const { tasks } = useWorkspaceRoute();
  useEffect(() => {
    if (tasks.project || tasks.task || !props.projects.length) return;
    void navigate({
      ...taskLocation({ ...tasks, project: props.projects[0] }),
      replace: true,
    });
  }, [
    navigate,
    props.projects,
    tasks.project,
    tasks.task,
    tasks.assignee,
    tasks.run,
  ]);
  return (
    <TasksView
      {...props}
      selection={tasks}
      select={(search) => void navigate(taskLocation(search))}
    />
  );
}
