import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { useProject } from "#src/shared/api/use-project.ts";
import { resourceHandle } from "#src/shared/navigation/resource-route.ts";
import { useEffect, useRef } from "react";
import type { useWorkspaceRoute } from "./router";

// Keep identity rather than an alias so project renames survive page changes.
export function useWorkspaceProject(
  c: Connection,
  projects: readonly Project[],
  route: ReturnType<typeof useWorkspaceRoute>,
  session?: Session,
) {
  const remembered = useRef<{ connection: Connection; id: string }>(undefined);
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
  const resolved = useProject(c, projects, selected);
  // These list pages display the first project when opened without a scope.
  // An unscoped task detail can instead require an explicit project choice.
  const defaultsToFirst =
    route.resource === "projects" ||
    (route.resource === "tasks" &&
      (!route.tasks.task || projects.length === 1));
  const active = selected || defaultsToFirst ? resolved : undefined;
  const id = active && projectId(active);
  useEffect(() => {
    if (id) remembered.current = { connection: c, id };
  }, [c, id]);
  const retained =
    remembered.current?.connection === c
      ? projects.find((p) => projectId(p) === remembered.current?.id)
      : undefined;
  const project = active ?? retained;
  return project && resourceHandle(project);
}
