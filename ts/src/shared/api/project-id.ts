import { Id, parse } from "@lesomnus/payday/pdid";
import type { Project } from "#gen/cxz/project_pb";

export const projectId = (project?: Pick<Project, "id">) =>
  project ? new Id(project.id).toString() : "";

export function projectRef(value: string) {
  try {
    return { key: { case: "id" as const, value: parse(value).bytes } };
  } catch {
    // Saved links from releases before project UUIDs became canonical.
    return { key: { case: "runtimeId" as const, value } };
  }
}
