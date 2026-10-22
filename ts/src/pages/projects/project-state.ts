import { ContainerState } from "#gen/cxz/project_pb";
import type { Project } from "#gen/cxz/project_pb";

// What a project's container is doing. It is derived by the server from the
// container inventory on every read and stored nowhere, so `UNSPECIFIED` is a
// real answer -- nothing was observed -- and not the same as `ABSENT`.
// See docs/plans/project-lifecycle-api.md.
// The names are message keys, so they are the words the catalogue has: a
// container that is `CREATED` has not been started, and `ABSENT` is there being
// no container rather than a container in some absent state.
const names = {
  [ContainerState.UNSPECIFIED]: "Unknown",
  [ContainerState.ABSENT]: "No container",
  [ContainerState.CREATED]: "Not started",
  [ContainerState.RUNNING]: "Running",
  [ContainerState.STOPPED]: "Stopped",
} as const satisfies Record<ContainerState, string>;

export const containerState = (project: Project) =>
  project.status?.containerState ?? ContainerState.UNSPECIFIED;
export const containerStateName = (
  state: ContainerState,
): (typeof names)[ContainerState] => names[state] ?? "Unknown";
export const running = (project: Project) =>
  containerState(project) === ContainerState.RUNNING;
