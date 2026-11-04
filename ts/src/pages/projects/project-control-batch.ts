import { ContainerState, type Project } from "#gen/cxz/project_pb";
import { containerState } from "./project-state";

export const projectOperations = ["stop", "start", "remove", "erase"] as const;
export type ProjectOperation = (typeof projectOperations)[number];
export function canControlProject(project: Project, kind: ProjectOperation) {
  const state = containerState(project);
  return (
    kind === "erase" ||
    (kind === "stop"
      ? state === ContainerState.RUNNING
      : kind === "start"
        ? state !== ContainerState.RUNNING &&
          state !== ContainerState.UNSPECIFIED
        : state !== ContainerState.ABSENT &&
          state !== ContainerState.UNSPECIFIED)
  );
}
export type ProjectOutcome = { project: Project } & (
  | { ok: true }
  | { ok: false; error: unknown }
);
export async function settleProjects(
  projects: readonly Project[],
  run: (project: Project) => Promise<void>,
) {
  const results: ProjectOutcome[] = new Array(projects.length);
  let next = 0;
  async function worker() {
    while (next < projects.length) {
      const index = next++,
        project = projects[index]!;
      try {
        await run(project);
        results[index] = { project, ok: true };
      } catch (error) {
        results[index] = { project, ok: false, error };
      }
    }
  }
  await Promise.all(
    Array.from({ length: Math.min(3, projects.length) }, worker),
  );
  return results;
}
