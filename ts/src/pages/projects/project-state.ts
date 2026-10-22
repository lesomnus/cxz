import { ContainerState } from "#gen/cxz/project_pb";
import type { Project } from "#gen/cxz/project_pb";

// What a project's container is doing. It is derived by the server from the
// container inventory on every read and stored nowhere, so `UNSPECIFIED` is a
// real answer -- nothing was observed -- and not the same as `ABSENT`.
// See docs/plans/project-lifecycle-api.md.
const names: Record<ContainerState, string> = {
  [ContainerState.UNSPECIFIED]: "Unknown",
  [ContainerState.ABSENT]: "Absent",
  [ContainerState.CREATED]: "Created",
  [ContainerState.RUNNING]: "Running",
  [ContainerState.STOPPED]: "Stopped",
};

export const containerState = (project: Project) =>
  project.status?.containerState ?? ContainerState.UNSPECIFIED;
export const containerStateName = (state: ContainerState) =>
  names[state] ?? "Unknown";
export const running = (project: Project) =>
  containerState(project) === ContainerState.RUNNING;
