import { Id, parse } from "@lesomnus/payday/pdid";
import type { Project } from "#gen/cxz/project_pb";
import { resourceRef, uuid } from "#src/shared/navigation/resource-route.ts";

export const projectId = (project?: Pick<Project, "id">) =>
  project ? new Id(project.id).toString() : "";

export const kubernetesWorkspace = "/workspace";

export function projectWorkspace(project?: Project) {
  if (!project) return undefined;
  return project.id.length === 16 && project.workspace === projectId(project)
    ? project.status?.remoteWorkspace || kubernetesWorkspace
    : project.workspace;
}

export function projectRef(value: string) {
  const id = uuid(value);
  if (id) return { key: { case: "id" as const, value: id } };
  try {
    return { key: { case: "id" as const, value: parse(value).bytes } };
  } catch {
    // Saved links from releases before project UUIDs became canonical.
    return /^[\da-f]{24}$/i.test(value)
      ? { key: { case: "runtimeId" as const, value } }
      : resourceRef(value);
  }
}
