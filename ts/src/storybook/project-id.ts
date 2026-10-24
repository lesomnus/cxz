import type { MessageInitShape } from "@bufbuild/protobuf";
import type { Project } from "#gen/cxz/project_pb";
import type { ProjectRefSchema } from "#gen/cxz/project_svc_pb";
import { projectId } from "#src/shared/api/project-id.ts";

export function projectFixtureId(index = 1) {
  const id = new Uint8Array(16);
  id[6] = 0x80;
  id[8] = 0x80;
  id[9] = 7;
  new DataView(id.buffer).setUint32(12, index);
  return id;
}

export function matchesProject(
  project: Project,
  ref?: MessageInitShape<typeof ProjectRefSchema>,
  legacy?: string,
) {
  const key = ref?.key;
  if (key?.case === "id")
    return (
      project.id.length === key.value.length &&
      project.id.every((b, i) => b === key.value[i])
    );
  if (key?.case === "alias") return key.value === project.alias;
  return (
    key?.value === projectId(project) || (!!legacy && key?.value === legacy)
  );
}
