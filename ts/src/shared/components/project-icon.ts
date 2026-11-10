import type { Project } from "#gen/cxz/project_pb";
import {
  projectIconDefinitions,
  type ProjectIconName,
} from "#src/shared/icons/icon.tsx";

// A project's icon is a label, not a field: it is how a client chooses to show
// the project, and cxz stores it without reading a meaning out of it. That
// means nothing validates it on the way in -- an older build, a hand-edited
// row or an agent can leave any string here -- so the name is checked on the
// way out, every time, and anything unrecognized is simply no icon.
export const projectIconLabel = "icon";

export function projectIconName(
  project: Pick<Project, "labels"> | undefined,
): ProjectIconName | undefined {
  const name = project?.labels?.[projectIconLabel];
  // Own properties only. `"constructor" in definitions` is true of every object
  // there is, and a label saying so would resolve to a function where a glyph
  // belongs -- which is a blank icon at best and a thrown render at worst.
  return name && Object.hasOwn(projectIconDefinitions, name)
    ? (name as ProjectIconName)
    : undefined;
}

/**
 * The labels to save for a project whose icon is being set, or cleared with
 * `undefined`.
 *
 * A patch replaces the whole map, so this returns the complete set rather than
 * the one entry: a project that grows another label later keeps it. Clearing
 * writes an empty value instead of dropping the key, because a patch carrying
 * an empty map is a patch that changes nothing -- which is the one thing a
 * person clearing an icon does not mean.
 */
export function withProjectIcon(
  labels: Readonly<Record<string, string>> | undefined,
  name: ProjectIconName | undefined,
): Record<string, string> {
  return { ...labels, [projectIconLabel]: name ?? "" };
}
