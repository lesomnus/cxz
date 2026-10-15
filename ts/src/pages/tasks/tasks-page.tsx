import { useNavigate } from "@tanstack/react-router";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { TasksView } from "#src/features/tasks/tasks-view.tsx";

export function TasksPage(props: {
  c: Connection;
  projects: Project[];
  sessions: Session[];
  projectsLoading?: boolean;
  sessionsLoading?: boolean;
}) {
  const navigate = useNavigate();
  const { tasks } = useWorkspaceRoute();
  return (
    <TasksView
      {...props}
      selection={tasks}
      select={(search) => void navigate({ to: "/tasks", search })}
    />
  );
}
