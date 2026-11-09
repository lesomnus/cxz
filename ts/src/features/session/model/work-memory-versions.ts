import { create } from "@bufbuild/protobuf";
import { key } from "@lesomnus/payday/store";
import {
  MemoryVersionSchema,
  type MemoryVersion,
} from "#gen/cxz/memory_svc_pb";
import type { Memory } from "#gen/cxz/memory_pb";
import { projectId, projectRef } from "#src/shared/api/project-id.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import type { WorkMemorySource } from "./work-memories";

const metadataTTL = 60_000;
export const initialMemoryVersion = (memory: Memory) =>
  memory.revision === 1n
    ? create(MemoryVersionSchema, { memoryId: memory.id, revision: 1n })
    : undefined;

/** Fetch only revision metadata, once per displayed revision, with bounded fan-out. */
export async function loadWorkMemoryVersions(
  source: WorkMemorySource,
  memories: readonly Memory[],
  signal: AbortSignal,
) {
  const result = new Map<string, MemoryVersion>();
  const pending = memories.filter((memory) => memory.revision > 1n);
  const project = projectId(source.project);
  let index = 0;
  await Promise.all(
    Array.from({ length: Math.min(4, pending.length) }, async () => {
      while (index < pending.length) {
        signal.throwIfAborted();
        const memory = pending[index++];
        const id = key(memory.id);
        try {
          const version = await source.cache.read(
            `memory-version/${project}/${id}/${memory.revision}`,
            metadataTTL,
            async (signal) => {
              const page = await source.memories.versions(
                {
                  ref: {
                    key: { case: "id", value: memory.id },
                    project: projectRef(project),
                  },
                  beforeRevision: memory.revision + 1n,
                  size: 1,
                },
                { signal, timeoutMs: 15_000 },
              );
              return page.versions.find(
                (version) => version.revision === memory.revision,
              );
            },
            signal,
          );
          if (version) result.set(id, version);
        } catch (error) {
          if (signal.aborted) throw error;
          // A missing/inaccessible revision stays unknown instead of becoming zero.
        }
      }
    }),
  );
  return result;
}

export function useWorkMemoryVersions(
  source: WorkMemorySource,
  memories: readonly Memory[],
) {
  const revisions = memories
    .map((memory) => `${key(memory.id)}/${memory.revision}`)
    .join(",");
  return useRemote(
    (signal) => loadWorkMemoryVersions(source, memories, signal),
    [source, revisions],
    {
      cache: {
        store: source.cache,
        key: `memory-menu-versions/${projectId(source.project)}/${revisions}`,
        ttl: metadataTTL,
      },
    },
  );
}
