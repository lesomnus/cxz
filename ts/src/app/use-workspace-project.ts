import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { useProject } from "#src/shared/api/use-project.ts";
import { resourceHandle } from "#src/shared/navigation/resource-route.ts";
import { useResourceSelection } from "#src/shared/api/resource-selection.ts";
import { sessionUUID } from "#src/shared/api/session-id.ts";
import { useEffect } from "react";
import type { useWorkspaceRoute } from "./router";

// Keep identity rather than an alias so project renames survive page changes.
export function useWorkspaceProject(
  c: Connection,
  projects: readonly Project[],
  route: ReturnType<typeof useWorkspaceRoute>,
  session?: Session,
) {
  const selection = useResourceSelection(c);
  const retained = projects.find((p) => projectId(p) === selection.project);
  const selected =
    route.resource === "sessions"
      ? session?.project && projectId(session.project)
      : route.resource === "projects"
        ? route.project
        : route.resource === "tasks"
          ? route.tasks.project
          : route.resource === "memory"
            ? route.memoryProject
            : undefined;
  const resolved = useProject(
    c,
    projects,
    selected || (retained && projectId(retained)),
  );
  // These scoped pages display the first project when opened without a scope.
  // An unscoped task detail can instead require an explicit project choice.
  const defaultsToFirst =
    route.resource === "memory" ||
    (route.resource === "tasks" &&
      (!route.tasks.task || projects.length === 1));
  const active = selected || defaultsToFirst ? resolved : undefined;
  const id = active && projectId(active);
  useEffect(() => {
    if (!id) return;
    if (route.resource === "sessions" && session)
      c.selection.chooseSession(id, sessionUUID(session.id), session.runtimeId);
    else c.selection.chooseProject(id);
  }, [
    c,
    id,
    route.resource,
    session && sessionUUID(session.id),
    session?.runtimeId,
  ]);
  const project = active ?? retained;
  return project && resourceHandle(project);
}
