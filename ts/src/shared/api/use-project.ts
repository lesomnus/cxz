import type { Project } from "#gen/cxz/project_pb";
import type { Connection } from "./connection";
import { projectId, projectRef } from "./project-id";
import { useRemote } from "./use-remote";

export function useProject(
  c: Connection,
  projects: readonly Project[],
  selected?: string,
) {
  const found =
    projects.find((p) => projectId(p) === selected) ??
    (!selected ? projects[0] : undefined);
  const legacy = useRemote(
    async (signal) => {
      if (!selected || found || projectRef(selected).key.case === "id")
        return undefined;
      return {
        handle: selected,
        project: await c.projects.get(
          { ref: projectRef(selected), select: { all: true } },
          { signal, timeoutMs: 10000 },
        ),
      };
    },
    [c, selected, !!found],
  );
  if (found) return found;
  const resolved = legacy.data;
  if (!resolved || resolved.handle !== selected) return undefined;
  return projects.find((p) => projectId(p) === projectId(resolved.project));
}
