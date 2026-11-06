import { ContainerState, type Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { key } from "@lesomnus/payday/store";

export function backgroundAvailable(
  session: Session | undefined,
  projects: readonly Project[],
) {
  if (!session?.project?.id.length || session.status?.state === "stopped")
    return false;
  const id = key(session.project.id);
  return projects.some(
    (project) =>
      key(project.id) === id &&
      project.status?.containerState === ContainerState.RUNNING,
  );
}
