import type { Project } from "#gen/cxz/project_pb";
import type { Connection } from "./connection";
import { projectId, projectRef } from "./project-id";
import { useRemote } from "./use-remote";
import { matchesResource } from "#src/shared/navigation/resource-route.ts";
import { useRef } from "react";

export function useProject(
  c: Connection,
  projects: readonly Project[],
  selected?: string,
) {
  const remembered = useRef<{ handle: string; id: string } | undefined>(
    undefined,
  );
  const found =
    projects.find((p) => selected && matchesResource(p, selected)) ??
    (remembered.current?.handle === selected
      ? projects.find((p) => projectId(p) === remembered.current?.id)
      : undefined) ??
    (!selected ? projects[0] : undefined);
  if (found && selected)
    remembered.current = { handle: selected, id: projectId(found) };
  const legacy = useRemote(
    async (signal) => {
      if (!selected || found) return undefined;
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
