import { useEffect } from "react";
import type { Memory } from "#gen/cxz/memory_pb";
import { MemoryScope } from "#gen/cxz/memory_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { projectRef } from "#src/shared/api/project-id.ts";
import { MEMORY_LIST_TTL, memoryListKey } from "#src/shared/api/read-cache.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { resourceHandle } from "#src/shared/navigation/resource-route.ts";

export function useProjectMemories(
  c: Connection,
  project: string,
  active = true,
  session = "",
) {
  const cacheKey = memoryListKey(project);
  const list = useRemote(
    async (signal) => {
      const items = new Map<string, Memory>();
      const cursors = new Set<string>();
      let after = "";
      do {
        const page = await c.memories.list(
          {
            project: projectRef(project),
            scope: MemoryScope.ACTIVE,
            size: 500,
            after,
          },
          { signal, timeoutMs: 30000 },
        );
        for (const item of page.items)
          items.set(resourceHandle({ id: item.id }), item);
        if (page.next && cursors.has(page.next))
          throw new Error("Invalid memory page cursor");
        after = page.next;
        cursors.add(after);
      } while (after);
      return {
        project,
        items: [...items.values()].sort(
          (a, b) =>
            Number(
              (b.dateUpdated?.seconds ?? 0n) - (a.dateUpdated?.seconds ?? 0n),
            ) ||
            (b.dateUpdated?.nanos ?? 0) - (a.dateUpdated?.nanos ?? 0) ||
            a.name.localeCompare(b.name) ||
            resourceHandle({ id: a.id }).localeCompare(
              resourceHandle({ id: b.id }),
            ),
        ),
      };
    },
    [c, project],
    {
      enabled: active && !!project,
      cache: { store: c.reads, key: cacheKey, ttl: MEMORY_LIST_TTL },
    },
  );
  useEffect(() => {
    if (active && project && !list.loading && !c.reads.peek(cacheKey)?.fresh)
      list.revalidate();
  }, [c, project, session, active]);
  const data = list.data?.project === project ? list.data : undefined;
  return { ...list, loaded: !!data, memories: data?.items ?? [] };
}

export type MemoryCatalog = ReturnType<typeof useProjectMemories>;
