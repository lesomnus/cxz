import type { Client } from "@connectrpc/connect";
import { parseAlias } from "@lesomnus/payday/slug";
import type { Project } from "#gen/cxz/project_pb";
import type { ProjectService } from "#gen/cxz/project_svc_pb";
import { t } from "#src/shared/i18n/i18n.ts";

export type ProjectField = "name" | "alias" | "desc";

export function validateProjectField(field: ProjectField, value: string) {
  if (field === "name") {
    if (
      !value.trim() ||
      new TextEncoder().encode(value).length > 200 ||
      /[\r\n\t\0]/.test(value)
    )
      return t(
        "Display name must be nonempty, at most 200 bytes and contain no control whitespace.",
      );
  } else if (field === "alias") {
    try {
      parseAlias(value);
    } catch (error) {
      return (error as Error).message;
    }
  }
}

export async function editProject(
  client: Pick<Client<typeof ProjectService>, "get" | "patch">,
  project: Project,
  field: ProjectField,
  value: string,
) {
  const invalid = validateProjectField(field, value);
  if (invalid) throw new Error(invalid);
  const ref = { key: { case: "id" as const, value: project.id } };
  const options = { signal: AbortSignal.timeout(20_000) };
  const current = project.dateUpdated
    ? project
    : await client.get({ ref, select: { all: true } }, options);
  if (!current.dateUpdated)
    throw new Error(
      t("Project version is unavailable. Refresh and try again."),
    );
  return client.patch(
    {
      ref,
      dateUpdated: current.dateUpdated,
      [field]: field === "alias" ? parseAlias(value) : value,
    },
    options,
  );
}
